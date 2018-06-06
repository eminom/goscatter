package data

type WorkNode struct {
	value interface{}
	prev  *WorkNode
	next  *WorkNode
}

type WorkNodeList struct {
	head WorkNode
	tail WorkNode
}

func NewWorkNodeList() *WorkNodeList {
	var rv WorkNodeList
	rv.head.next = &rv.tail
	rv.tail.prev = &rv.head
	return &rv
}

func (n *WorkNodeList) Walk(d func(interface{})) {
	for ptr := n.head.next; ptr != &n.tail; ptr = ptr.next {
		d(ptr.value)
	}
}

func (n *WorkNodeList) Append(v interface{}) {
	node := &WorkNode{
		value: v,
		prev:  n.tail.prev,
		next:  &n.tail,
	}
	n.tail.prev.next = node
	n.tail.prev = node
}

func (n *WorkNodeList) Remove(v interface{}) {
	var nodeToDel *WorkNode
	for ptr := n.head.next; ptr != &n.tail; ptr = ptr.next {
		if ptr.value == v {
			nodeToDel = ptr
			break
		}
	}
	if nodeToDel != nil {
		n.RemoveNode(nodeToDel)
	}
}

func (n *WorkNodeList) RemoveNode(nd *WorkNode) {
	if nd == &n.head || nd == &n.tail {
		panic("No way")
	}
	nd.prev.next = nd.next
	nd.next.prev = nd.prev
}

func (n *WorkNodeList) Shift() interface{} {
	if n.head.next == &n.tail {
		return nil
	}
	rv := n.head.next
	n.RemoveNode(rv)
	return rv.value
}

func (n *WorkNodeList) Pop() interface{} {
	if n.head.next == &n.tail {
		return nil
	}
	rv := n.tail.prev
	n.RemoveNode(rv)
	return rv.value
}
