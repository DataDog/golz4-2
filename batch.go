package lz4

type batchIter struct {
	count, size, Start, End int
}

// Batch returns an iterator that will set its Start and End to
// size batches til count items are reached.  Usage:
//
// b := Batch(len(slice), batchSize)
// for b.Next() {
//    shard := slice[b.Start:b.End]
//    ...
// }
//
func batch(count, size int) *batchIter {
	return &batchIter{count: count, size: size}
}

// Next will return true if there is another batch to process using the updated b.Start and b.End indices
//
// b := Batch(len(slice), batchSize)
// for b.Next() {
//    shard := slice[b.Start:b.End]
//    ...
// }
//
func (b *batchIter) Next() bool {
	if b.End == b.count {
		return false
	}

	b.Start = b.End
	b.End += b.size

	if b.End > b.count {
		b.End = b.count
	}
	return true
}
