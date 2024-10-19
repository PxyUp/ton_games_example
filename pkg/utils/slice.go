package utils

func SliceRemoveWithoutSaveOrder[T any](slice []T, index int) []T {
	slice[index] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}
