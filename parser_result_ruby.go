package gotreesitter

func normalizeRubyTopLevelModuleBounds(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ruby" || root.Type(lang) != "program" || len(source) == 0 {
		return
	}
	end := lastNonTriviaByteEnd(source)
	for _, child := range root.children {
		if child == nil || child.IsExtra() || child.Type(lang) != "module" {
			continue
		}
		if len(child.children) > 0 && child.children[0] != nil && child.startByte < child.children[0].startByte {
			child.startByte = child.children[0].startByte
			child.startPoint = child.children[0].startPoint
		}
		if child.endByte == root.endByte && end > child.startByte && end < child.endByte {
			child.endByte = end
			child.endPoint = advancePointByBytes(Point{}, source[:end])
		}
	}
}

func normalizeRubyThenStarts(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ruby" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		switch n.Type(lang) {
		case "elsif", "if", "unless", "when":
			normalizeRubyThenChildStarts(n, lang)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeRubyThenChildStarts(parent *Node, lang *Language) {
	if parent == nil || lang == nil || len(parent.children) < 2 {
		return
	}
	for i, child := range parent.children {
		if child == nil || child.Type(lang) != "then" || i == 0 {
			continue
		}
		prev := (*Node)(nil)
		for j := i - 1; j >= 0; j-- {
			if parent.children[j] != nil {
				prev = parent.children[j]
				break
			}
		}
		if prev == nil || prev.endByte >= child.startByte {
			continue
		}
		child.startByte = prev.endByte
		child.startPoint = prev.endPoint
	}
}
