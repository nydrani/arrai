package syntax

import (
	"github.com/arr-ai/arrai/rel"
)

func stdFmt() rel.Attr {
	return rel.NewTupleAttr(
		"fmt",
		rel.NewNativeFunctionAttr("pretty", func(value rel.Value) (rel.Value, error) {
			prettifiedString, err := PrettifyString(value, 0)
			if err != nil {
				return nil, err
			}

			return rel.NewString([]rune(prettifiedString)), nil
		}),
	)
}
