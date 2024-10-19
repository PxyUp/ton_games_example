package http_server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/database"
	"github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/google/uuid"

	"io"
	"log"
	"math/big"
	"net/http"
	"time"

	"github.com/PxyUp/ton_games_example/pkg/ton"
	"github.com/golang-jwt/jwt"
	echo "github.com/labstack/echo/v4"
	"github.com/tonkeeper/tongo"
	"github.com/tonkeeper/tongo/abi"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/liteapi"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/wallet"
)

const (
	tonProofPrefix   = "ton-proof-item-v2/"
	tonConnectPrefix = "ton-connect"
)

type Domain struct {
	LengthBytes uint32 `json:"lengthBytes"`
	Value       string `json:"value"`
}

type MessageInfo struct {
	Timestamp int64  `json:"timestamp"`
	Domain    Domain `json:"domain"`
	Signature string `json:"signature"`
	Payload   string `json:"payload"`
	StateInit string `json:"state_init"`
}

type TonProof struct {
	Address string      `json:"address"`
	Network string      `json:"network"`
	Proof   MessageInfo `json:"proof"`
}

type ParsedMessage struct {
	Workchain int32
	Address   []byte
	Timstamp  int64
	Domain    Domain
	Signature []byte
	Payload   string
	StateInit string
}

type Payload struct {
	ExpirtionTime int64
	Signature     string
}

type JwtCustomClaims struct {
	UserId string `json:"userId"`
	jwt.StandardClaims
}

type Handlers interface {
	CheckProof(req *http.Request) (database.AccountRecord, error)
	GenerateProof(req *http.Request) (string, error)
	GetUserFromCtx(c echo.Context) (database.AccountRecord, error)
	GetAccountInfo(ctx context.Context, token *jwt.Token) (database.AccountRecord, error)
}

type handlers struct {
	pprof     ton.Pprof
	gameStore database.DB
	logger    logger.Logger
}

var networks = map[string]*liteapi.Client{}

var knownHashes = make(map[string]wallet.Version)

func init() {
	for i := wallet.Version(0); i <= wallet.HighLoadV2R2; i++ {
		ver := wallet.GetCodeHashByVer(i)
		knownHashes[hex.EncodeToString(ver[:])] = i
	}

	var err error
	networks["-239"], err = liteapi.NewClientWithDefaultMainnet()
	if err != nil {
		log.Fatal(err)
	}

	networks["-3"], err = liteapi.NewClientWithDefaultTestnet()
	if err != nil {
		log.Fatal(err)
	}
}

func (h *handlers) GetAccountInfo(ctx context.Context, token *jwt.Token) (database.AccountRecord, error) {
	claims := token.Claims.(*JwtCustomClaims)

	uuidUser, err := uuid.Parse(claims.UserId)
	if err != nil {
		return nil, err
	}

	return h.gameStore.GetPlayerById(ctx, uuidUser)
}

func (h *handlers) CheckProof(req *http.Request) (database.AccountRecord, error) {
	defer func() {
		if req != nil && req.Body != nil {
			req.Body.Close()
		}
	}()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		h.logger.Errorw("cant read request body", "error", err.Error())
		return nil, err
	}
	tp := &TonProof{}
	err = json.Unmarshal(b, tp)
	if err != nil {
		h.logger.Errorw("cant unmarshals request body", "error", err.Error())
		return nil, err
	}

	err = h.pprof.Check(tp.Proof.Payload)
	if err != nil {
		h.logger.Errorw("cant validate proof by payload", "error", err.Error())
		return nil, err
	}

	parsed, err := convertTonProofMessage(tp)
	if err != nil {
		h.logger.Errorw("cant convert to message", "error", err.Error())
		return nil, err
	}

	net := networks[tp.Network]
	if net == nil {
		h.logger.Errorw("invalid network should -239 or -1")
		return nil, fmt.Errorf("invalid network")
	}

	addr, err := tongo.ParseAddress(tp.Address)
	if err != nil {
		h.logger.Errorw("cant parse ton address")
		return nil, err
	}

	check, err := checkProof(addr.ID, net, parsed)
	if err != nil {
		h.logger.Errorw("cant validate proof by data from msg and network", "error", err.Error())
		return nil, err
	}

	if !check {
		return nil, fmt.Errorf("invalid pprof")
	}

	record, err := h.gameStore.GetPlayerByAddress(req.Context(), addr.ID.String())
	if err == nil {
		return record, nil
	}

	if errors.Is(err, database.ErrMissingPlayer) {
		return h.gameStore.CreatePlayer(req.Context(), addr.ID.String())
	}

	return nil, err
}

func (h *handlers) GetUserFromCtx(c echo.Context) (database.AccountRecord, error) {
	value := c.Get("user")
	if value == nil {
		return nil, fmt.Errorf("cant get user from ctx")
	}

	token, ok := value.(*jwt.Token)
	if !ok {
		return nil, fmt.Errorf("cant get jwt token")
	}
	user, err := h.GetAccountInfo(c.Request().Context(), token)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (h *handlers) GenerateProof(req *http.Request) (string, error) {
	return h.pprof.Generate()
}

func New(pprof ton.Pprof, log logger.Logger, gameStore database.DB) Handlers {
	return &handlers{
		pprof:     pprof,
		gameStore: gameStore,
		logger:    log.With("component", "ton_api_handlers"),
	}
}

func convertTonProofMessage(tp *TonProof) (*ParsedMessage, error) {
	addr, err := tongo.ParseAddress(tp.Address)
	if err != nil {
		return nil, err
	}

	var parsedMessage ParsedMessage

	sig, err := base64.StdEncoding.DecodeString(tp.Proof.Signature)
	if err != nil {
		return nil, err
	}

	parsedMessage.Workchain = addr.ID.Workchain
	parsedMessage.Address = addr.ID.Address[:]
	parsedMessage.Domain = tp.Proof.Domain
	parsedMessage.Timstamp = tp.Proof.Timestamp
	parsedMessage.Signature = sig
	parsedMessage.Payload = tp.Proof.Payload
	parsedMessage.StateInit = tp.Proof.StateInit
	return &parsedMessage, nil
}

func createMessage(message *ParsedMessage) ([]byte, error) {
	wc := make([]byte, 4)
	binary.BigEndian.PutUint32(wc, uint32(message.Workchain))

	ts := make([]byte, 8)
	binary.LittleEndian.PutUint64(ts, uint64(message.Timstamp))

	dl := make([]byte, 4)
	binary.LittleEndian.PutUint32(dl, message.Domain.LengthBytes)
	m := []byte(tonProofPrefix)
	m = append(m, wc...)
	m = append(m, message.Address...)
	m = append(m, dl...)
	m = append(m, []byte(message.Domain.Value)...)
	m = append(m, ts...)
	m = append(m, []byte(message.Payload)...)
	messageHash := sha256.Sum256(m)
	fullMes := []byte{0xff, 0xff}
	fullMes = append(fullMes, []byte(tonConnectPrefix)...)
	fullMes = append(fullMes, messageHash[:]...)
	res := sha256.Sum256(fullMes)
	return res[:], nil
}

func checkProof(address tongo.AccountID, net *liteapi.Client, tonProofReq *ParsedMessage) (bool, error) {
	pubKey, err := getWalletPubKey(address, net)
	if err != nil {
		if tonProofReq.StateInit == "" {
			return false, err
		}
		if ok, err := compareStateInitWithAddress(address, tonProofReq.StateInit); err != nil || !ok {
			return ok, err
		}
		pubKey, err = parseStateInit(tonProofReq.StateInit)
		if err != nil {
			return false, err
		}
	}

	if time.Now().After(time.Unix(tonProofReq.Timstamp, 0).Add(time.Duration(config.Config.ProofLifeTimeSec) * time.Second)) {
		msgErr := "proof has been expired"
		return false, errors.New(msgErr)
	}

	if tonProofReq.Domain.Value != config.Config.ExampleDomain {
		msgErr := fmt.Sprintf("wrong domain: %v", tonProofReq.Domain)
		return false, errors.New(msgErr)
	}

	mes, err := createMessage(tonProofReq)
	if err != nil {
		return false, err
	}

	return signatureVerify(pubKey, mes, tonProofReq.Signature), nil
}

func getWalletPubKey(address tongo.AccountID, net *liteapi.Client) (ed25519.PublicKey, error) {
	_, result, err := abi.GetPublicKey(context.Background(), net, address)
	if err != nil {
		return nil, err
	}
	if r, ok := result.(abi.GetPublicKeyResult); ok {
		i := big.Int(r.PublicKey)
		b := i.Bytes()
		if len(b) < 24 || len(b) > 32 { // govno kakoe-to
			return nil, fmt.Errorf("invalid publock key")
		}
		return append(make([]byte, 32-len(b)), b...), nil // make padding if first bytes are empty
	}
	return nil, fmt.Errorf("can't get publick key")
}

func compareStateInitWithAddress(a tongo.AccountID, stateInit string) (bool, error) {
	cells, err := boc.DeserializeBocBase64(stateInit)
	if err != nil || len(cells) != 1 {
		return false, err
	}
	h, err := cells[0].Hash()
	if err != nil {
		return false, err
	}
	return bytes.Equal(h, a.Address[:]), nil
}

func parseStateInit(stateInit string) ([]byte, error) {
	cells, err := boc.DeserializeBocBase64(stateInit)
	if err != nil || len(cells) != 1 {
		return nil, err
	}
	var state tlb.StateInit
	err = tlb.Unmarshal(cells[0], &state)
	if err != nil {
		return nil, err
	}
	if !state.Data.Exists || !state.Code.Exists {
		return nil, fmt.Errorf("empty init state")
	}
	codeHash, err := state.Code.Value.Value.HashString()
	if err != nil {
		return nil, err
	}
	version, prs := knownHashes[codeHash]
	if !prs {
		return nil, fmt.Errorf("unknown code hash")
	}
	var pubKey tlb.Bits256
	switch version {
	case wallet.V1R1, wallet.V1R2, wallet.V1R3, wallet.V2R1, wallet.V2R2:
		var data wallet.DataV1V2
		err = tlb.Unmarshal(&state.Data.Value.Value, &data)
		if err != nil {
			return nil, err
		}
		pubKey = data.PublicKey
	case wallet.V3R1, wallet.V3R2, wallet.V4R1, wallet.V4R2:
		var data wallet.DataV3
		err = tlb.Unmarshal(&state.Data.Value.Value, &data)
		if err != nil {
			return nil, err
		}
		pubKey = data.PublicKey
	case wallet.V5Beta:
		var data wallet.DataV5Beta
		err = tlb.Unmarshal(&state.Data.Value.Value, &data)
		if err != nil {
			return nil, err
		}
		pubKey = data.PublicKey
	case wallet.V5R1:
		var data wallet.DataV5R1
		err = tlb.Unmarshal(&state.Data.Value.Value, &data)
		if err != nil {
			return nil, err
		}
		pubKey = data.PublicKey
	}

	return pubKey[:], nil
}

func signatureVerify(pubkey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(pubkey, message, signature)
}
