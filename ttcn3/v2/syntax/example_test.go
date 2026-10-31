package syntax_test

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/v2/syntax"
)

func main() {
	root := syntax.Parse([]byte("1+2*3"))
	root.Inspect(func(n syntax.Node) bool {
		fmt.Print(n.Kind().String())
		return true
	})
}
