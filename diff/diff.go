package diff

type DiffOperation struct {
	Op      Operation
	Literal string
}

func GetDiffString(a string, b string, lcs string) []DiffOperation {
	var diffs []DiffOperation
	var i, j, k int
	for i < len(a) || j < len(b) {

		if i < len(a) && j < len(b) && k < len(lcs) && a[i] == lcs[k] && b[j] == lcs[k] {
			// both are equal to the lcs, keep as is
			diffs = append(diffs, DiffOperation{Op: KEEP, Literal: string(a[i])})
			i++
			j++
			k++
		} else if j < len(b) && (k == len(lcs) || b[j] != lcs[k]) {
			// if edited is diff from lcs, its an addiion

			diffs = append(diffs, DiffOperation{Op: ADD, Literal: string(b[j])})
			j++
		} else if i < len(a) && (k == len(lcs) || a[i] != lcs[k]) {
			// if a doesnt match lcs, its a delection
			diffs = append(diffs, DiffOperation{Op: DEL, Literal: string(a[i])})
			i++
		}

	}

	return diffs
}

func GetDiffTokens(a []string, b []string, lcs []string) []DiffOperation {
	var diffs []DiffOperation
	var i, j, k int
	for i < len(a) || j < len(b) {

		if i < len(a) && j < len(b) && k < len(lcs) && a[i] == lcs[k] && b[j] == lcs[k] {
			// both are equal to the lcs, keep as is
			diffs = append(diffs, DiffOperation{Op: KEEP, Literal: string(a[i])})
			i++
			j++
			k++
		} else if j < len(b) && (k == len(lcs) || b[j] != lcs[k]) {
			// if edited is diff from lcs, its an addiion

			diffs = append(diffs, DiffOperation{Op: ADD, Literal: string(b[j])})
			j++
		} else if i < len(a) && (k == len(lcs) || a[i] != lcs[k]) {
			// if a doesnt match lcs, its a delection
			diffs = append(diffs, DiffOperation{Op: DEL, Literal: string(a[i])})
			i++
		}

	}

	return diffs
}
