package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type dfaBenchmarkSpec struct {
	lang   func() *gotreesitter.Language
	source func(int) []byte
	marker string
}

func makeTypeScriptBenchmarkSource(funcCount int) []byte {
	const lineLen = len("export function f0(): number { const v = 0; return v }\n")
	buf := make([]byte, 0, funcCount*lineLen)
	for i := 0; i < funcCount; i++ {
		buf = append(buf, []byte("export function f")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("(): number { const v = ")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("; return v }\n")...)
	}
	return buf
}

func makePythonBenchmarkSource(funcCount int) []byte {
	const lineLen = len("def f0():\n    v = 0\n    return v\n\n")
	buf := make([]byte, 0, funcCount*lineLen)
	for i := 0; i < funcCount; i++ {
		buf = append(buf, []byte("def f")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("():\n    v = ")...)
		buf = append(buf, []byte(stringInt(i))...)
		buf = append(buf, []byte("\n    return v\n\n")...)
	}
	return buf
}

func benchmarkParseFullDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	src := spec.source(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		requireCompleteParse(b, tree, src, lang, "full dfa")
		tree.Release()
	}
}

func benchmarkParseIncrementalSingleByteEditDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	src := spec.source(benchmarkFuncCount(b))
	sites := makeBenchmarkEditSites(src, spec.marker)
	if len(sites) == 0 {
		b.Fatalf("could not find edit marker %q", spec.marker)
	}
	site := sites[0]

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(site.offset),
		OldEndByte:  uint32(site.offset + 1),
		NewEndByte:  uint32(site.offset + 1),
		StartPoint:  site.start,
		OldEndPoint: site.end,
		NewEndPoint: site.end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		toggleDigitAt(src, site.offset)
		tree.Edit(edit)
		old := tree
		tree, err = parser.ParseIncremental(src, tree)
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
	}
	tree.Release()
}

func benchmarkParseIncrementalNoEditDFA(b *testing.B, spec dfaBenchmarkSpec) {
	lang := spec.lang()
	parser := gotreesitter.NewParser(lang)
	src := spec.source(benchmarkFuncCount(b))

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("initial parse error: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		old := tree
		tree, err = parser.ParseIncremental(src, tree)
		if err != nil {
			b.Fatalf("incremental parse error: %v", err)
		}
		if tree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		if old != tree {
			old.Release()
		}
	}
	tree.Release()
}

func stringInt(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func BenchmarkTypeScriptParseFullDFA(b *testing.B) {
	benchmarkParseFullDFA(b, dfaBenchmarkSpec{
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkTypeScriptParseIncrementalSingleByteEditDFA(b *testing.B) {
	benchmarkParseIncrementalSingleByteEditDFA(b, dfaBenchmarkSpec{
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkTypeScriptParseIncrementalNoEditDFA(b *testing.B) {
	benchmarkParseIncrementalNoEditDFA(b, dfaBenchmarkSpec{
		lang:   grammars.TypescriptLanguage,
		source: makeTypeScriptBenchmarkSource,
		marker: "const v = ",
	})
}

func BenchmarkPythonParseFullDFA(b *testing.B) {
	benchmarkParseFullDFA(b, dfaBenchmarkSpec{
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}

func BenchmarkPythonParseIncrementalSingleByteEditDFA(b *testing.B) {
	benchmarkParseIncrementalSingleByteEditDFA(b, dfaBenchmarkSpec{
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}

func BenchmarkPythonParseIncrementalNoEditDFA(b *testing.B) {
	benchmarkParseIncrementalNoEditDFA(b, dfaBenchmarkSpec{
		lang:   grammars.PythonLanguage,
		source: makePythonBenchmarkSource,
		marker: "v = ",
	})
}
