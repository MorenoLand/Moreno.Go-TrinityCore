// C++ owner: src/tools/vmap4_extractor/gameobject_extract.cpp
// (ExtractGameobjectModels, ExtractSingleModel).
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// extractGameobjectModels mirrors C++ ExtractGameobjectModels: read
// DBFilesClient\GameObjectDisplayInfo.dbc (field 0 = display id, field 1
// = model path), extract each model's file (.wmo via ExtractSingleWmo,
// anything else via ExtractSingleModel; .mdl is a C++ TODO and skipped),
// and append (displayId, isWmo, pathLength, path) per success to
// temp_gameobject_models, a VMAP047-magic index the vmap4assembler
// consumes. A missing or malformed DBC is fatal, exactly like C++.
func (e *extractor) extractGameobjectModels() error {
	fmt.Print("Extracting GameObject models...")
	data, err := e.readMPQFile(`DBFilesClient\GameObjectDisplayInfo.dbc`)
	if err != nil {
		return fmt.Errorf("Fatal error: Invalid GameObjectDisplayInfo.dbc file format!")
	}
	dbc, err := openDBC(data)
	if err != nil {
		return fmt.Errorf("Fatal error: Invalid GameObjectDisplayInfo.dbc file format: %w", err)
	}
	listPath := filepath.Join(e.output, "temp_gameobject_models")
	list, err := os.Create(listPath)
	if err != nil {
		return fmt.Errorf("Fatal error: Could not open file %s", listPath)
	}
	defer list.Close()
	if _, err := list.WriteString("VMAP047"); err != nil {
		return err
	}
	if _, err := list.Write([]byte{0}); err != nil {
		return err
	}
	for id := uint32(0); id < dbc.records(); id++ {
		path, err := dbc.getString(id, 1)
		if err != nil {
			continue
		}
		if len(path) < 4 {
			continue
		}
		fixed := []byte(path)
		fixnamen(fixed)
		name := fixedPlainName(string(fixed))
		ext := getExtension(name)
		if ext == "" {
			continue
		}
		var result bool
		var isWmo uint8
		switch strings.ToLower(ext) {
		case ".wmo":
			isWmo = 1
			result = e.extractSingleWmo(path)
		case ".mdl":
			// C++ owner: "TODO: extract .mdl files, if needed" — skipped.
			continue
		default:
			result = e.extractSingleModel(path)
		}
		if !result {
			continue
		}
		displayID, err := dbc.getUInt(id, 0)
		if err != nil {
			continue
		}
		write := func(v any) error { return binary.Write(list, binary.LittleEndian, v) }
		if err := write(displayID); err != nil {
			return err
		}
		if err := write(isWmo); err != nil {
			return err
		}
		if err := write(uint32(len(name))); err != nil {
			return err
		}
		if _, err := list.WriteString(name); err != nil {
			return err
		}
	}
	fmt.Println("Done!")
	return nil
}
