package util

// Batch 将切片按固定大小拆分，最后一批可能小于 batchSize。
func Batch[T any](items []T, batchSize int) [][]T {
	if batchSize <= 0 {
		batchSize = len(items)
		if batchSize == 0 {
			return nil
		}
	}
	var result [][]T
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		chunk := make([]T, end-start)
		copy(chunk, items[start:end])
		result = append(result, chunk)
	}
	return result
}
