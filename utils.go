package main

func keys(d map[uint64]bool) []uint64 {
	var keys []uint64
	for key := range d {
		keys = append(keys, key)
	}
	return keys
}

