package grammars

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the properties grammar.
const (
	propertiesTokEof = 0
)

const (
	propertiesSymEof gotreesitter.Symbol = 16
)

// PropertiesExternalScanner handles EOF detection for Java .properties files.
type PropertiesExternalScanner struct{}

func (PropertiesExternalScanner) Create() any                           { return nil }
func (PropertiesExternalScanner) Destroy(payload any)                   {}
func (PropertiesExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (PropertiesExternalScanner) Deserialize(payload any, buf []byte)   {}

func (PropertiesExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !propertiesValid(validSymbols, propertiesTokEof) {
		return false
	}
	if lexer.Lookahead() == 0 {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(propertiesSymEof)
		return true
	}
	return false
}

func propertiesValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
