# Tagger, BoundTree, Walk, ParseFile Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add four features to gotreesitter: a Tagger (parallel to Highlighter), BoundTree convenience wrapper, Walk helper, and ParseFile one-liner.

**Architecture:** Each feature is a new file in the root `gotreesitter` package (or `grammars` for ParseFile). Tagger mirrors Highlighter's pattern: compile query at construction, parse+execute on each call. BoundTree binds a Tree to its Language to eliminate the `lang` parameter from every node call. Walk is a generic DFS traversal with skip/stop control. ParseFile ties together detection+parsing in one call.

**Tech Stack:** Pure Go, zero new dependencies. Uses existing `gotreesitter` types.

---

### Task 1: Walk helper

**Files:**
- Create: `walk.go`
- Test: `walk_test.go`

**Step 1: Write the failing test**

```go
// walk_test.go
package gotreesitter

import "testing"

func TestWalkDFS(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang) // program > [func_decl > [func_kw, ident, number]]

	var visited []string
	Walk(tree.RootNode(), func(node *Node, depth int) WalkAction {
		visited = append(visited, node.Type(lang))
		return WalkContinue
	})

	if len(visited) == 0 {
		t.Fatal("Walk visited no nodes")
	}
	// Root should be first
	if visited[0] != "program" {
		t.Errorf("first visited = %q, want %q", visited[0], "program")
	}
}

func TestWalkSkipChildren(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	var visited []string
	Walk(tree.RootNode(), func(node *Node, depth int) WalkAction {
		visited = append(visited, node.Type(lang))
		if node.Type(lang) == "function_declaration" {
			return WalkSkipChildren
		}
		return WalkContinue
	})

	// Should visit program and function_declaration but not its children
	for _, name := range visited {
		if name == "identifier" || name == "number" {
			t.Errorf("visited %q despite WalkSkipChildren on function_declaration", name)
		}
	}
}

func TestWalkStop(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	count := 0
	Walk(tree.RootNode(), func(node *Node, depth int) WalkAction {
		count++
		return WalkStop
	})

	if count != 1 {
		t.Errorf("visited %d nodes, want 1 (WalkStop after first)", count)
	}
}

func TestWalkNilNode(t *testing.T) {
	// Should not panic
	Walk(nil, func(node *Node, depth int) WalkAction {
		t.Fatal("should not be called")
		return WalkContinue
	})
}

func TestWalkDepth(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)

	maxDepth := 0
	Walk(tree.RootNode(), func(node *Node, depth int) WalkAction {
		if depth > maxDepth {
			maxDepth = depth
		}
		return WalkContinue
	})

	// program(0) > func_decl(1) > [func_kw(2), ident(2), number(2)]
	if maxDepth < 2 {
		t.Errorf("maxDepth = %d, want >= 2", maxDepth)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/draco/work/gotreesitter && go test -run TestWalk -v`
Expected: FAIL — `Walk`, `WalkAction`, etc. are undefined.

**Step 3: Write minimal implementation**

```go
// walk.go
package gotreesitter

// WalkAction controls the tree walk behavior.
type WalkAction int

const (
	// WalkContinue continues the walk to children and siblings.
	WalkContinue WalkAction = iota
	// WalkSkipChildren skips the current node's children but continues to siblings.
	WalkSkipChildren
	// WalkStop terminates the walk entirely.
	WalkStop
)

// Walk performs a depth-first traversal of the syntax tree rooted at node.
// The callback receives each node and its depth (0 for the starting node).
// Return WalkSkipChildren to skip a node's children, or WalkStop to end early.
func Walk(node *Node, fn func(node *Node, depth int) WalkAction) {
	if node == nil {
		return
	}

	type entry struct {
		node  *Node
		depth int
	}

	stack := []entry{{node: node, depth: 0}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		action := fn(e.node, e.depth)
		if action == WalkStop {
			return
		}
		if action == WalkSkipChildren {
			continue
		}

		children := e.node.Children()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, entry{node: children[i], depth: e.depth + 1})
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/draco/work/gotreesitter && go test -run TestWalk -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/draco/work/gotreesitter && git add walk.go walk_test.go && buckley commit --yes --minimal-output
```

---

### Task 2: BoundTree wrapper

**Files:**
- Create: `bound_tree.go`
- Test: `bound_tree_test.go`

**Step 1: Write the failing test**

```go
// bound_tree_test.go
package gotreesitter

import "testing"

func TestBoundTreeNodeType(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)

	root := bt.RootNode()
	if got := bt.NodeType(root); got != "program" {
		t.Errorf("NodeType(root) = %q, want %q", got, "program")
	}
}

func TestBoundTreeNodeText(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("func main 42")
	funcKw := leaf(Symbol(8), false, 0, 4)
	ident := leaf(Symbol(1), true, 5, 9)
	num := leaf(Symbol(2), true, 10, 12)
	program := parent(Symbol(7), true,
		[]*Node{funcKw, ident, num},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, source, lang)
	bt := Bind(tree)

	if got := bt.NodeText(ident); got != "main" {
		t.Errorf("NodeText(ident) = %q, want %q", got, "main")
	}
}

func TestBoundTreeChildByField(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildFieldedTree(lang) // func_decl with name field
	bt := Bind(tree)

	root := bt.RootNode()
	funcDecl := root.Child(0)
	nameNode := bt.ChildByField(funcDecl, "name")
	if nameNode == nil {
		t.Fatal("ChildByField(name) returned nil")
	}
	if got := bt.NodeType(nameNode); got != "identifier" {
		t.Errorf("ChildByField(name) type = %q, want %q", got, "identifier")
	}
}

func TestBoundTreeRelease(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)
	bt.Release() // should not panic
	bt.Release() // double release should not panic
}

func TestBoundTreeLanguage(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildSimpleTree(lang)
	bt := Bind(tree)

	if bt.Language() != lang {
		t.Error("Language() returned wrong language")
	}
}

func TestBoundTreeSource(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("func main 42")
	funcKw := leaf(Symbol(8), false, 0, 4)
	ident := leaf(Symbol(1), true, 5, 9)
	num := leaf(Symbol(2), true, 10, 12)
	program := parent(Symbol(7), true,
		[]*Node{funcKw, ident, num},
		[]FieldID{0, 0, 0})
	tree := NewTree(program, source, lang)
	bt := Bind(tree)

	if string(bt.Source()) != "func main 42" {
		t.Errorf("Source() = %q, want %q", string(bt.Source()), "func main 42")
	}
}

func TestBindNil(t *testing.T) {
	bt := Bind(nil)
	if bt.RootNode() != nil {
		t.Error("Bind(nil).RootNode() should be nil")
	}
	bt.Release() // should not panic
}
```

This needs a `buildFieldedTree` helper. Add to `query_test.go` (or create a test helper).

**Step 2: Run test to verify it fails**

Run: `cd /home/draco/work/gotreesitter && go test -run TestBoundTree -v`
Expected: FAIL — `Bind`, `BoundTree` are undefined.

**Step 3: Write minimal implementation**

```go
// bound_tree.go
package gotreesitter

// BoundTree pairs a Tree with its Language and source, eliminating the need
// to pass *Language and []byte to every node method call.
type BoundTree struct {
	tree *Tree
}

// Bind creates a BoundTree from a Tree. The Tree must have been created with
// a Language (via NewTree or a Parser). Returns a BoundTree that delegates to
// the underlying Tree's Language and Source.
func Bind(tree *Tree) *BoundTree {
	return &BoundTree{tree: tree}
}

// RootNode returns the tree's root node.
func (bt *BoundTree) RootNode() *Node {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.RootNode()
}

// Language returns the tree's language.
func (bt *BoundTree) Language() *Language {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.Language()
}

// Source returns the tree's source bytes.
func (bt *BoundTree) Source() []byte {
	if bt == nil || bt.tree == nil {
		return nil
	}
	return bt.tree.Source()
}

// NodeType returns the node's type name, resolved via the bound language.
func (bt *BoundTree) NodeType(n *Node) string {
	if bt == nil || bt.tree == nil || n == nil {
		return ""
	}
	return n.Type(bt.tree.Language())
}

// NodeText returns the source text covered by the node.
func (bt *BoundTree) NodeText(n *Node) string {
	if bt == nil || bt.tree == nil || n == nil {
		return ""
	}
	return n.Text(bt.tree.Source())
}

// ChildByField returns the first child assigned to the given field name.
func (bt *BoundTree) ChildByField(n *Node, fieldName string) *Node {
	if bt == nil || bt.tree == nil || n == nil {
		return nil
	}
	return n.ChildByFieldName(fieldName, bt.tree.Language())
}

// Release releases the underlying tree's arena memory.
func (bt *BoundTree) Release() {
	if bt == nil || bt.tree == nil {
		return
	}
	bt.tree.Release()
}
```

Also add `buildFieldedTree` to test helpers (a separate step below).

**Step 4: Add test helper and run tests**

Add `buildFieldedTree` to `query_test.go`:

```go
// buildFieldedTree creates a tree with field annotations:
// program > function_declaration(name: identifier)
func buildFieldedTree(lang *Language) *Tree {
	source := []byte("func main 42")
	ident := leaf(Symbol(1), true, 5, 9) // identifier "main"
	// function_declaration with name field on the identifier
	funcDecl := parent(Symbol(3), true,
		[]*Node{ident},
		[]FieldID{1}) // fieldID 1 = "name"
	program := parent(Symbol(7), true,
		[]*Node{funcDecl},
		[]FieldID{0})
	return NewTree(program, source, lang)
}
```

Run: `cd /home/draco/work/gotreesitter && go test -run TestBoundTree -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/draco/work/gotreesitter && git add bound_tree.go bound_tree_test.go query_test.go && buckley commit --yes --minimal-output
```

---

### Task 3: Tagger (Tags query support)

**Files:**
- Create: `tagger.go`
- Test: `tagger_test.go`

**Step 1: Write the failing test**

```go
// tagger_test.go
package gotreesitter

import "testing"

func TestTaggerBasic(t *testing.T) {
	lang := queryTestLanguage()

	tagger, err := NewTagger(lang, `
(function_declaration name: (identifier) @name) @definition.function
(identifier) @name @reference.call
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tree := buildFieldedTree(lang) // program > func_decl(name: ident)
	tags := tagger.TagTree(tree)

	if len(tags) == 0 {
		t.Fatal("expected tags, got none")
	}

	// Should have a definition.function tag
	found := false
	for _, tag := range tags {
		if tag.Kind == "definition.function" {
			found = true
			if tag.Name == "" {
				t.Error("definition.function tag has empty Name")
			}
		}
	}
	if !found {
		t.Errorf("no definition.function tag found in %+v", tags)
	}
}

func TestTaggerEmptySource(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `(identifier) @name @reference.call`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags := tagger.Tag(nil)
	if tags != nil {
		t.Errorf("expected nil for nil source, got %+v", tags)
	}

	tags = tagger.Tag([]byte{})
	if tags != nil {
		t.Errorf("expected nil for empty source, got %+v", tags)
	}
}

func TestTaggerInvalidQuery(t *testing.T) {
	lang := queryTestLanguage()
	_, err := NewTagger(lang, `(nonexistent_node) @name @definition.function`)
	if err == nil {
		t.Fatal("expected error for invalid query")
	}
}

func TestTaggerWithTokenSourceFactory(t *testing.T) {
	lang := queryTestLanguage()
	factoryCalled := false
	factory := func(source []byte) TokenSource {
		factoryCalled = true
		return &eofTokenSource{pos: uint32(len(source))}
	}

	tagger, err := NewTagger(lang, `(identifier) @name @definition.function`,
		WithTaggerTokenSourceFactory(factory))
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tagger.Tag([]byte("test"))
	if !factoryCalled {
		t.Error("expected token source factory to be called")
	}
}

func TestTaggerTagTree(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `
(function_declaration) @definition.function
(identifier) @name
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tree := buildSimpleTree(lang)
	tags := tagger.TagTree(tree)

	// Should tag something
	if len(tags) == 0 {
		t.Fatal("expected tags from TagTree, got none")
	}
}

func TestTaggerIncremental(t *testing.T) {
	lang := queryTestLanguage()
	tagger, err := NewTagger(lang, `(identifier) @name @definition.function`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	// TagIncremental with nil oldTree should work like Tag
	tags, tree := tagger.TagIncremental([]byte("test"), nil)
	_ = tags
	if tree == nil {
		t.Fatal("TagIncremental returned nil tree")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/draco/work/gotreesitter && go test -run TestTagger -v`
Expected: FAIL — `Tag`, `Tagger`, `NewTagger` are undefined.

**Step 3: Write minimal implementation**

```go
// tagger.go
package gotreesitter

// Tag represents a tagged symbol in source code, extracted by a Tagger.
// Kind follows tree-sitter convention: "definition.function", "reference.call", etc.
// Name is the captured symbol text (e.g., the function name).
type Tag struct {
	Kind      string // e.g. "definition.function", "reference.call"
	Name      string // the captured symbol text
	Range     Range  // full span of the tagged node
	NameRange Range  // span of the @name capture
}

// Tagger extracts symbol definitions and references from source code using
// tree-sitter tags queries. It is the tagging counterpart to Highlighter.
//
// Tags queries use a convention where captures follow the pattern:
//   - @name captures the symbol name (e.g., function identifier)
//   - @definition.X or @reference.X captures the kind
//
// Example query:
//
//	(function_declaration name: (identifier) @name) @definition.function
//	(call_expression function: (identifier) @name) @reference.call
type Tagger struct {
	parser             *Parser
	query              *Query
	lang               *Language
	tokenSourceFactory func(source []byte) TokenSource
}

// TaggerOption configures a Tagger.
type TaggerOption func(*Tagger)

// WithTaggerTokenSourceFactory sets a factory function that creates a TokenSource
// for each Tag call.
func WithTaggerTokenSourceFactory(factory func(source []byte) TokenSource) TaggerOption {
	return func(tg *Tagger) {
		tg.tokenSourceFactory = factory
	}
}

// NewTagger creates a Tagger for the given language and tags query.
func NewTagger(lang *Language, tagsQuery string, opts ...TaggerOption) (*Tagger, error) {
	q, err := NewQuery(tagsQuery, lang)
	if err != nil {
		return nil, err
	}

	tg := &Tagger{
		parser: NewParser(lang),
		query:  q,
		lang:   lang,
	}
	for _, opt := range opts {
		opt(tg)
	}
	return tg, nil
}

// Tag parses source and returns all tags.
func (tg *Tagger) Tag(source []byte) []Tag {
	if len(source) == 0 {
		return nil
	}

	tree := tg.parse(source, nil)
	if tree.RootNode() == nil {
		return nil
	}
	defer tree.Release()

	return tg.tagTree(tree)
}

// TagTree extracts tags from an already-parsed tree.
func (tg *Tagger) TagTree(tree *Tree) []Tag {
	if tree == nil || tree.RootNode() == nil {
		return nil
	}
	return tg.tagTree(tree)
}

// TagIncremental re-tags source after edits to oldTree.
// Returns the tags and the new tree for subsequent incremental calls.
func (tg *Tagger) TagIncremental(source []byte, oldTree *Tree) ([]Tag, *Tree) {
	if len(source) == 0 {
		return nil, NewTree(nil, source, tg.lang)
	}

	tree := tg.parse(source, oldTree)
	if tree.RootNode() == nil {
		return nil, tree
	}

	return tg.tagTree(tree), tree
}

func (tg *Tagger) parse(source []byte, oldTree *Tree) *Tree {
	if tg.tokenSourceFactory != nil {
		ts := tg.tokenSourceFactory(source)
		if oldTree != nil {
			return tg.parser.ParseIncrementalWithTokenSource(source, oldTree, ts)
		}
		return tg.parser.ParseWithTokenSource(source, ts)
	}
	if oldTree != nil {
		return tg.parser.ParseIncremental(source, oldTree)
	}
	return tg.parser.Parse(source)
}

func (tg *Tagger) tagTree(tree *Tree) []Tag {
	matches := tg.query.Execute(tree)
	if len(matches) == 0 {
		return nil
	}

	var tags []Tag
	for _, m := range matches {
		tag := tg.extractTag(m, tree.Source())
		if tag.Kind != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// extractTag converts a query match into a Tag. It looks for a @name capture
// and a @definition.* or @reference.* capture within the match.
func (tg *Tagger) extractTag(m QueryMatch, source []byte) Tag {
	var tag Tag
	for _, c := range m.Captures {
		switch {
		case c.Name == "name":
			tag.Name = c.Node.Text(source)
			tag.NameRange = c.Node.Range()
		case len(c.Name) > 11 && c.Name[:11] == "definition." ||
			len(c.Name) > 10 && c.Name[:10] == "reference.":
			tag.Kind = c.Name
			tag.Range = c.Node.Range()
		}
	}
	// If we got a kind but no separate name, use the kind node's text
	if tag.Kind != "" && tag.Name == "" {
		tag.Name = string(source[tag.Range.StartByte:tag.Range.EndByte])
		tag.NameRange = tag.Range
	}
	// If we got a name but no kind, use the full match range
	if tag.Name != "" && tag.Kind == "" && tag.Range.EndByte == 0 {
		// No kind capture — skip this match
		return Tag{}
	}
	return tag
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/draco/work/gotreesitter && go test -run TestTagger -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/draco/work/gotreesitter && git add tagger.go tagger_test.go && buckley commit --yes --minimal-output
```

---

### Task 4: Add TagsQuery to LangEntry and ParseFile

**Files:**
- Modify: `grammars/registry.go` (add TagsQuery field, ParseFile function)
- Test: `grammars/parse_file_test.go`

**Step 1: Write the failing test**

```go
// grammars/parse_file_test.go
package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestParseFile(t *testing.T) {
	bt, err := ParseFile("main.go", []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	root := bt.RootNode()
	if root == nil {
		t.Fatal("ParseFile returned nil root")
	}
	if got := bt.NodeType(root); got != "source_file" {
		t.Errorf("root type = %q, want %q", got, "source_file")
	}
}

func TestParseFileUnknownExtension(t *testing.T) {
	_, err := ParseFile("file.xyz", []byte("hello"))
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
}

func TestParseFileEmptySource(t *testing.T) {
	bt, err := ParseFile("main.go", []byte{})
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	if bt.RootNode() != nil && bt.RootNode().ChildCount() > 0 {
		// Empty source may produce empty tree; that's fine
	}
}

func TestParseFilePython(t *testing.T) {
	bt, err := ParseFile("script.py", []byte("def hello():\n    pass\n"))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	if bt.RootNode() == nil {
		t.Fatal("ParseFile returned nil root for Python")
	}

	// Walk the tree to find a function_definition
	found := false
	gotreesitter.Walk(bt.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "function_definition" {
			found = true
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	if !found {
		t.Error("expected to find function_definition in Python parse tree")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/draco/work/gotreesitter && go test -run TestParseFile -v ./grammars/`
Expected: FAIL — `ParseFile` is undefined.

**Step 3: Modify registry.go and implement ParseFile**

Add `TagsQuery` field to `LangEntry`:

```go
// In registry.go, add TagsQuery to LangEntry:
type LangEntry struct {
	Name               string
	Extensions         []string
	Shebangs           []string
	Language           func() *gotreesitter.Language
	HighlightQuery     string
	TagsQuery          string  // tree-sitter tags.scm query for symbol extraction
	TokenSourceFactory func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource
}
```

Create `grammars/parse_file.go`:

```go
// grammars/parse_file.go
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/draco/work/gotreesitter && go test -run TestParseFile -v ./grammars/`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/draco/work/gotreesitter && git add grammars/registry.go grammars/parse_file.go grammars/parse_file_test.go && buckley commit --yes --minimal-output
```

---

### Task 5: Integration test with real Go grammar

**Files:**
- Create: `tagger_integration_test.go`

**Step 1: Write integration test**

```go
// tagger_integration_test.go
package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTaggerIntegrationGo(t *testing.T) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		t.Skip("Go grammar not available")
	}

	lang := entry.Language()
	source := []byte(`package main

func Add(a, b int) int {
	return a + b
}

func main() {
	Add(1, 2)
}
`)

	tagger, err := gotreesitter.NewTagger(lang, `
(function_declaration name: (identifier) @name) @definition.function
(method_declaration name: (field_identifier) @name) @definition.method
(call_expression function: (identifier) @name) @reference.call
`)
	if err != nil {
		t.Fatalf("NewTagger error: %v", err)
	}

	tags := tagger.Tag(source)
	if len(tags) == 0 {
		t.Fatal("expected tags from Go source")
	}

	// Check we found both function definitions
	defs := 0
	refs := 0
	for _, tag := range tags {
		switch {
		case tag.Kind == "definition.function":
			defs++
			t.Logf("def: %s at %d:%d", tag.Name, tag.NameRange.StartPoint.Row, tag.NameRange.StartPoint.Column)
		case tag.Kind == "reference.call":
			refs++
			t.Logf("ref: %s at %d:%d", tag.Name, tag.NameRange.StartPoint.Row, tag.NameRange.StartPoint.Column)
		}
	}

	if defs < 2 {
		t.Errorf("expected >= 2 function definitions, got %d", defs)
	}
	if refs < 1 {
		t.Errorf("expected >= 1 call reference, got %d", refs)
	}
}

func TestParseFileAndWalkIntegration(t *testing.T) {
	bt, err := grammars.ParseFile("main.go", []byte(`package main

func hello() {}
`))
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	defer bt.Release()

	var funcNames []string
	gotreesitter.Walk(bt.RootNode(), func(node *gotreesitter.Node, depth int) gotreesitter.WalkAction {
		if bt.NodeType(node) == "function_declaration" {
			nameNode := bt.ChildByField(node, "name")
			if nameNode != nil {
				funcNames = append(funcNames, bt.NodeText(nameNode))
			}
		}
		return gotreesitter.WalkContinue
	})

	if len(funcNames) != 1 || funcNames[0] != "hello" {
		t.Errorf("expected [hello], got %v", funcNames)
	}
}
```

**Step 2: Run integration test**

Run: `cd /home/draco/work/gotreesitter && go test -run TestTaggerIntegration -v && go test -run TestParseFileAndWalk -v`
Expected: PASS

**Step 3: Commit**

```bash
cd /home/draco/work/gotreesitter && git add tagger_integration_test.go && buckley commit --yes --minimal-output
```
