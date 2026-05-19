package tokens

func Estimate(sizeBytes int64) int64 {
	if sizeBytes <= 0 {
		return 0
	}
	return sizeBytes / 4
}
