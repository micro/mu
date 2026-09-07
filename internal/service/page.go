package service

// PageRange bounds list responses to 100 records while retaining the total.
// Callers filter and sort before applying it.
func PageRange(total, offset, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return offset, end
}
