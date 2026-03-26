package grammargen

import "testing"

func TestBuildSeqCoalescesAdjacentStrings(t *testing.T) {
	builder := newNFABuilder()
	seqFrag, err := builder.buildFromRule(Seq(Str("a"), Str("b"), Str("cd")))
	if err != nil {
		t.Fatalf("buildFromRule(seq): %v", err)
	}
	seqStateCount := len(builder.states)

	builder = newNFABuilder()
	stringFrag, err := builder.buildFromRule(Str("abcd"))
	if err != nil {
		t.Fatalf("buildFromRule(string): %v", err)
	}
	stringStateCount := len(builder.states)

	if seqStateCount != stringStateCount {
		t.Fatalf("seq state count = %d, want %d", seqStateCount, stringStateCount)
	}
	if seqFrag.end-seqFrag.start != stringFrag.end-stringFrag.start {
		t.Fatalf("seq fragment width = %d, want %d", seqFrag.end-seqFrag.start, stringFrag.end-stringFrag.start)
	}
}
