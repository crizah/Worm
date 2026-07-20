package token

import (
	"bufio"
	"os"
	"strings"
)

// tokenise a string by lines

func TokeniseFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	// scanner.Scan() automatically stops at every newline
	for scanner.Scan() {
		str := strings.TrimSpace(scanner.Text())
		if str != "" { // get rid of empty lines too
			lines = append(lines, str)

		}

	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func tokeniseString(s string) []string {
	arr := strings.Split(s, "\n")
	for i, str := range arr {
		arr[i] = strings.TrimSpace(str)

	}

	return arr
}
