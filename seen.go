package main

type Seen struct {
	seen map[uint64]bool
}

func NewSeen() *Seen {
	m := new(Seen)
	m.seen = make(map[uint64]bool)
	return m
}

func (s Seen) Add(id uint64) {
	s.seen[id] = true
}

func (s Seen) Remove(id uint64) {
	delete(s.seen, id)
}

func (s Seen) HasKey(id uint64) bool {
	_, ok := s.seen[id]
	return ok
}

func (s Seen) Size() int {
	return len(s.seen)
}
