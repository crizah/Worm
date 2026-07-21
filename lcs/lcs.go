package lcs

type state struct {
	i, j int
}

func LCSstring(a string, b string) string {
	cache := make(map[state]string)
	return solve(a, b, 0, 0, cache)
}

func solve(a string, b string, i int, j int, cache map[state]string) string {
	if i == len(a) || j == len(b) {
		return ""
	}
	// check cache
	currState := state{i, j}
	if val, exists := cache[currState]; exists {
		return val
	}

	var result string

	if a[i] == b[j] {

		result = string(a[i]) + solve(a, b, i+1, j+1, cache)
	} else {

		resultA := solve(a, b, i+1, j, cache)
		resultB := solve(a, b, i, j+1, cache)

		if len(resultA) > len(resultB) {
			result = resultA
		} else {
			result = resultB
		}
	}

	cache[currState] = result
	return result

}

func LCSLines(a []string, b []string) []string {
	cache := make(map[state][]string)
	return solveLines(a, b, 0, 0, cache)
}

func solveLines(a []string, b []string, i int, j int, cache map[state][]string) []string {
	if i == len(a) || j == len(b) {
		return []string{}
	}

	currState := state{i, j}
	if val, exists := cache[currState]; exists {
		return val
	}

	var result []string

	// Compare whole lines, not characters
	if a[i] == b[j] {
		// matched

		result = append([]string{a[i]}, solveLines(a, b, i+1, j+1, cache)...)
	} else {
		resultA := solveLines(a, b, i+1, j, cache)
		resultB := solveLines(a, b, i, j+1, cache)

		if len(resultA) > len(resultB) {
			result = resultA
		} else {
			result = resultB
		}
	}

	cache[currState] = result
	return result
}
