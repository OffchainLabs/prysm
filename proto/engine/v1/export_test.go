package enginev1

type Copier[T any] copier[T]

func MarshalItems[T sszMarshaler](items []T) ([]byte, error) {
	return marshalItems(items)
}

func UnmarshalItems[T sszUnmarshaler](data []byte, itemSize int, newItem func() T) ([]T, error) {
	return unmarshalItems(data, itemSize, newItem)
}
