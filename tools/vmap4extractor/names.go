// C++ owner: src/tools/vmap4_extractor/adtfile.cpp
//   - GetPlainName (const char* + char* overloads), fixnamen, fixname2,
//     GetExtension. wdtfile.cpp's wdtGetPlainName is the same function.
package main

import "strings"

// plainName mirrors C++ GetPlainName: strip everything up to the last
// backslash. MPQ names use backslashes; a forward slash is NOT a separator
// here (unlike filepath.Base, which would keep the whole backslash path on
// Linux).
func plainName(fileName string) string {
	if idx := strings.LastIndexByte(fileName, '\\'); idx >= 0 {
		return fileName[idx+1:]
	}
	return fileName
}

// fixnamen mirrors C++ fixnamen: title-case the stem (upper-case the first
// letter of each alpha run, lower-case the rest) and force the last three
// characters (the extension) to lower case.
func fixnamen(name []byte) {
	if len(name) < 3 {
		return
	}
	for i := 0; i < len(name)-3; i++ {
		if i > 0 && name[i] >= 'A' && name[i] <= 'Z' && isAlpha(name[i-1]) {
			name[i] |= 0x20
		} else if (i == 0 || !isAlpha(name[i-1])) && name[i] >= 'a' && name[i] <= 'z' {
			name[i] &^= 0x20
		}
	}
	for i := len(name) - 3; i < len(name); i++ {
		name[i] |= 0x20
	}
}

func isAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// fixname2 mirrors C++ fixname2: spaces in the stem become underscores.
func fixname2(name []byte) {
	if len(name) < 3 {
		return
	}
	for i := 0; i < len(name)-3; i++ {
		if name[i] == ' ' {
			name[i] = '_'
		}
	}
}

// fixedPlainName is the ExtractSingleWmo / ExtractSingleModel output-name
// recipe: plain name, then fixnamen, then fixname2.
func fixedPlainName(fileName string) string {
	name := []byte(plainName(fileName))
	fixnamen(name)
	fixname2(name)
	return string(name)
}

// getExtension mirrors C++ GetExtension: pointer to the last '.' or "".
func getExtension(fileName string) string {
	if idx := strings.LastIndexByte(fileName, '.'); idx >= 0 {
		return fileName[idx:]
	}
	return ""
}
