package grammars

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// ParseFile detects the language from filename, parses source, and returns
// a BoundTree. The caller must call Release() on the returned BoundTree.
func ParseFile(filename string, source []byte) (*gotreesitter.BoundTree, error) {
	entry := DetectLanguage(filename)
	if entry == nil {
		return nil, fmt.Errorf("unsupported file type: %s", filename)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)

	var tree *gotreesitter.Tree
	if entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		tree = parser.ParseWithTokenSource(source, ts)
	} else {
		tree = parser.Parse(source)
	}

	return gotreesitter.Bind(tree), nil
}
