package tuple

type Tuple []any

func New(items ...any) Tuple {
	v := make(Tuple, len(items))
	copy(v, items)
	return v
}

func Get[T any](tup Tuple, idx int) T {
	return tup[idx].(T)
}

func Update[T any](tup Tuple, idx int, v any) Tuple {
	x := New(tup...)
	x[idx] = v
	return x
}

func Two[ONE any, TWO any](t Tuple) (ONE, TWO) {
	return Get[ONE](t, 0), Get[TWO](t, 1)
}
