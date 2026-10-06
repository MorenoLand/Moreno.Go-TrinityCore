// MorenoCore VMAP4 Tile Assembler: compiles building model geometry into runtime vmaps.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	rawVMapMagic  = "VMAP047"
	vMapMagic     = "VMAP_4.7"
	rawHeaderSize = 8
)

type vector3 struct{ X, Y, Z float32 }

type rawLiquid struct {
	TilesX, TilesY uint32
	Corner         vector3
	Type           uint32
	Heights        []float32
	Flags          []byte
}

type rawGroup struct {
	MogpFlags, GroupWMOID uint32
	Low, High             vector3
	Triangles             [][3]uint16
	Vertices              []vector3
	Liquid                *rawLiquid
}

type rawModel struct {
	RootWMOID uint32
	Groups    []rawGroup
}

func printBanner() {
	fmt.Println("==========================================================")
	fmt.Println(" MorenoCore VMAP4 Tile Assembler (Go Parity Version)")
	fmt.Println(" Compiles raw building model geometry into runtime vmaps")
	fmt.Println("==========================================================")
}

func main() {
	srcDir := flag.String("src", "Buildings", "Source directory containing raw building models")
	destDir := flag.String("dest", "vmaps", "Destination directory for compiled vmap files")
	flag.Parse()

	printBanner()

	src := *srcDir
	dest := *destDir

	if len(flag.Args()) > 0 {
		src = flag.Args()[0]
	}
	if len(flag.Args()) > 1 {
		dest = flag.Args()[1]
	}

	fmt.Printf("Using '%s' as source directory and writing output to '%s'\n", src, dest)

	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Source directory '%s' does not exist or is not a directory.\n", src)
		fmt.Fprintf(os.Stderr, "Please run vmap4extractor first to extract raw building models.\n")
		os.Exit(1)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create destination directory '%s': %v\n", dest, err)
		os.Exit(1)
	}

	start := time.Now()
	entries, err := os.ReadDir(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read source directory '%s': %v\n", src, err)
		os.Exit(1)
	}

	modelsProcessed := 0
	failed := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".wmo" || ext == ".m2" || ext == ".mdx" || ext == ".vmo" {
			data, err := os.ReadFile(filepath.Join(src, name))
			if err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "Failed to read raw model %q: %v\n", name, err)
				continue
			}
			if len(data) < len(rawVMapMagic) || string(data[:len(rawVMapMagic)]) != rawVMapMagic {
				failed = true
				fmt.Fprintf(os.Stderr, "Model %q is not a VMAP047 raw model; run the reference-compatible extractor first.\n", name)
				continue
			}
			model, err := readRawModel(bytes.NewReader(data))
			if err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "Failed to parse raw model %q: %v\n", name, err)
				continue
			}
			destFile := filepath.Join(dest, name+".vmo")
			file, err := os.Create(destFile)
			if err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "Failed to create %q: %v\n", destFile, err)
				continue
			}
			err = writeVMO(file, model)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				failed = true
				if err == nil {
					err = closeErr
				}
				fmt.Fprintf(os.Stderr, "Failed to write %q: %v\n", destFile, err)
				continue
			}
			modelsProcessed++
		}
	}
	if err := assembleMapTrees(src, dest); err != nil {
		failed = true
		fmt.Fprintf(os.Stderr, "Failed to assemble map trees: %v\n", err)
	}
	if err := exportGameobjectModels(src, dest); err != nil {
		failed = true
		fmt.Fprintf(os.Stderr, "Failed to export gameobject models: %v\n", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("Assembled %d building model trees into '%s' in %v\n", modelsProcessed, dest, elapsed.Round(time.Millisecond))
	if failed {
		fmt.Fprintln(os.Stderr, "VMAP assembly completed with errors")
		os.Exit(1)
	}
	fmt.Println("Ok, all done")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func readRawModel(reader io.Reader) (rawModel, error) {
	var model rawModel
	magic := make([]byte, rawHeaderSize)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return model, err
	}
	if string(magic[:len(rawVMapMagic)]) != rawVMapMagic || magic[len(rawVMapMagic)] != 0 {
		return model, errors.New("invalid VMAP047 raw model header")
	}
	var ignored, groups uint32
	if err := binary.Read(reader, binary.LittleEndian, &ignored); err != nil {
		return model, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &groups); err != nil {
		return model, err
	}
	if err := binary.Read(reader, binary.LittleEndian, &model.RootWMOID); err != nil {
		return model, err
	}
	if groups > 65535 {
		return model, errors.New("raw model group count is unreasonable")
	}
	model.Groups = make([]rawGroup, groups)
	for i := range model.Groups {
		group := &model.Groups[i]
		if err := binary.Read(reader, binary.LittleEndian, &group.MogpFlags); err != nil {
			return rawModel{}, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &group.GroupWMOID); err != nil {
			return rawModel{}, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &group.Low); err != nil {
			return rawModel{}, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &group.High); err != nil {
			return rawModel{}, err
		}
		var liquidFlags, branches uint32
		if err := binary.Read(reader, binary.LittleEndian, &liquidFlags); err != nil {
			return rawModel{}, err
		}
		if err := readChunkHeader(reader, "GRP "); err != nil {
			return rawModel{}, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &branches); err != nil {
			return rawModel{}, err
		}
		if branches > 1000000 || !discardUint32s(reader, branches) {
			return rawModel{}, errors.New("invalid raw model GRP branch table")
		}
		if err := readChunkHeader(reader, "INDX"); err != nil {
			return rawModel{}, err
		}
		var indexes uint32
		if err := binary.Read(reader, binary.LittleEndian, &indexes); err != nil || indexes%3 != 0 || indexes > 100000000 {
			return rawModel{}, errors.New("invalid raw model index count")
		}
		indexData := make([]uint16, indexes)
		if err := binary.Read(reader, binary.LittleEndian, &indexData); err != nil {
			return rawModel{}, err
		}
		group.Triangles = make([][3]uint16, indexes/3)
		for j := range group.Triangles {
			group.Triangles[j] = [3]uint16{indexData[j*3], indexData[j*3+1], indexData[j*3+2]}
		}
		if err := readChunkHeader(reader, "VERT"); err != nil {
			return rawModel{}, err
		}
		var vertices uint32
		if err := binary.Read(reader, binary.LittleEndian, &vertices); err != nil || vertices > 100000000 {
			return rawModel{}, errors.New("invalid raw model vertex count")
		}
		group.Vertices = make([]vector3, vertices)
		if err := binary.Read(reader, binary.LittleEndian, &group.Vertices); err != nil {
			return rawModel{}, err
		}
		if liquidFlags&3 == 0 {
			continue
		}
		if err := readChunkHeader(reader, "LIQU"); err != nil {
			return rawModel{}, err
		}
		liquid := &rawLiquid{}
		if err := binary.Read(reader, binary.LittleEndian, &liquid.Type); err != nil {
			return rawModel{}, err
		}
		if liquidFlags&1 != 0 {
			var xverts, yverts int32
			var xtiles, ytiles int32
			var material int16
			if err := binary.Read(reader, binary.LittleEndian, &xverts); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &yverts); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &xtiles); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &ytiles); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &liquid.Corner); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &material); err != nil {
				return rawModel{}, err
			}
			if xverts < 0 || yverts < 0 || xtiles < 0 || ytiles < 0 {
				return rawModel{}, errors.New("invalid raw model liquid header")
			}
			liquid.TilesX, liquid.TilesY = uint32(xtiles), uint32(ytiles)
			heightCount := int64(xverts) * int64(yverts)
			flagCount := int64(xtiles) * int64(ytiles)
			if heightCount > 100000000 || flagCount > 100000000 {
				return rawModel{}, errors.New("raw model liquid dimensions are unreasonable")
			}
			liquid.Heights = make([]float32, int(heightCount))
			liquid.Flags = make([]byte, int(flagCount))
			if err := binary.Read(reader, binary.LittleEndian, &liquid.Heights); err != nil {
				return rawModel{}, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &liquid.Flags); err != nil {
				return rawModel{}, err
			}
		} else {
			liquid.Heights = []float32{group.High.Z}
		}
		model.Groups[i].Liquid = liquid
	}
	return model, nil
}

func readChunkHeader(reader io.Reader, want string) error {
	chunk := make([]byte, 4)
	if _, err := io.ReadFull(reader, chunk); err != nil {
		return err
	}
	if string(chunk) != want {
		return fmt.Errorf("expected %q chunk, got %q", want, string(chunk))
	}
	var size uint32
	return binary.Read(reader, binary.LittleEndian, &size)
}

func discardUint32s(reader io.Reader, count uint32) bool {
	if count == 0 {
		return true
	}
	_, err := io.CopyN(io.Discard, reader, int64(count)*4)
	return err == nil
}

func writeVMO(writer io.Writer, model rawModel) error {
	if _, err := io.WriteString(writer, vMapMagic); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, "WMOD"); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(8)); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, model.RootWMOID); err != nil {
		return err
	}
	if len(model.Groups) == 0 {
		return nil
	}
	if _, err := io.WriteString(writer, "GMOD"); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(model.Groups))); err != nil {
		return err
	}
	for _, group := range model.Groups {
		if err := binary.Write(writer, binary.LittleEndian, group.Low); err != nil || binary.Write(writer, binary.LittleEndian, group.High) != nil || binary.Write(writer, binary.LittleEndian, group.MogpFlags) != nil || binary.Write(writer, binary.LittleEndian, group.GroupWMOID) != nil {
			return errors.New("failed to write VMAP group header")
		}
		if err := writeGroupGeometry(writer, group); err != nil {
			return err
		}
	}
	return writeGroupBIH(writer, groupBounds(model.Groups))
}

func writeGroupGeometry(writer io.Writer, group rawGroup) error {
	if _, err := io.WriteString(writer, "VERT"); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(4+len(group.Vertices)*12)); err != nil || binary.Write(writer, binary.LittleEndian, uint32(len(group.Vertices))) != nil || binary.Write(writer, binary.LittleEndian, group.Vertices) != nil {
		return errors.New("failed to write VMAP vertices")
	}
	if len(group.Vertices) == 0 {
		return nil
	}
	if _, err := io.WriteString(writer, "TRIM"); err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(4+len(group.Triangles)*12)); err != nil || binary.Write(writer, binary.LittleEndian, uint32(len(group.Triangles))) != nil {
		return errors.New("failed to write VMAP triangle header")
	}
	for _, triangle := range group.Triangles {
		for _, index := range triangle {
			if err := binary.Write(writer, binary.LittleEndian, uint32(index)); err != nil {
				return err
			}
		}
	}
	if err := writeBIHForGroup(writer, group); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, "LIQU"); err != nil {
		return err
	}
	if group.Liquid == nil {
		return binary.Write(writer, binary.LittleEndian, uint32(0))
	}
	liquidSize := uint32(24 + len(group.Liquid.Heights)*4 + len(group.Liquid.Flags))
	if err := binary.Write(writer, binary.LittleEndian, liquidSize); err != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.TilesX) != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.TilesY) != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.Corner) != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.Type) != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.Heights) != nil || binary.Write(writer, binary.LittleEndian, group.Liquid.Flags) != nil {
		return errors.New("failed to write VMAP liquid")
	}
	return nil
}

func writeBIHForGroup(writer io.Writer, group rawGroup) error {
	primitives := make([]bihPrimitive, len(group.Triangles))
	for index, triangle := range group.Triangles {
		if int(triangle[0]) >= len(group.Vertices) || int(triangle[1]) >= len(group.Vertices) || int(triangle[2]) >= len(group.Vertices) {
			return errors.New("triangle index is out of range")
		}
		low := minVector(group.Vertices[triangle[0]], minVector(group.Vertices[triangle[1]], group.Vertices[triangle[2]]))
		high := maxVector(group.Vertices[triangle[0]], maxVector(group.Vertices[triangle[1]], group.Vertices[triangle[2]]))
		primitives[index] = bihPrimitive{Low: low, High: high, Index: uint32(index)}
	}
	if _, err := io.WriteString(writer, "MBIH"); err != nil {
		return err
	}
	return writeBIH(writer, primitives)
}

func writeGroupBIH(writer io.Writer, bounds [][2]vector3) error {
	primitives := make([]bihPrimitive, len(bounds))
	for index, bound := range bounds {
		primitives[index] = bihPrimitive{Low: bound[0], High: bound[1], Index: uint32(index)}
	}
	if _, err := io.WriteString(writer, "GBIH"); err != nil {
		return err
	}
	return writeBIH(writer, primitives)
}

func groupBounds(groups []rawGroup) [][2]vector3 {
	result := make([][2]vector3, len(groups))
	for i, group := range groups {
		result[i] = [2]vector3{group.Low, group.High}
	}
	return result
}

func minVector(a, b vector3) vector3 {
	return vector3{X: float32(math.Min(float64(a.X), float64(b.X))), Y: float32(math.Min(float64(a.Y), float64(b.Y))), Z: float32(math.Min(float64(a.Z), float64(b.Z)))}
}
func maxVector(a, b vector3) vector3 {
	return vector3{X: float32(math.Max(float64(a.X), float64(b.X))), Y: float32(math.Max(float64(a.Y), float64(b.Y))), Z: float32(math.Max(float64(a.Z), float64(b.Z)))}
}
