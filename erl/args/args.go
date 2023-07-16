package args

import "github.com/uberbrodt/erl-go/erl/tuple"

type Args struct {
	items map[string]any
}

func New(args ...tuple.Tuple) Args {
	i := make(map[string]any)

	for _, t := range args {
		k, v := tuple.Two[string, any](t)
		i[k] = v
	}

	return Args{items: i}
}

func (args *Args) Add(k string, v any) *Args {
	args.items[k] = v

	return args
}

func Get[T any](args Args, key string) T {
	return args.items[key].(T)
}
