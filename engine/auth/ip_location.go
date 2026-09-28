package auth

import (
	"encoding/binary"
	"encoding/csv"
	"errors"
	"io"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
)

type ipLocationRange struct {
	from, to uint32
	country  string
}

func countryLockMismatch(ipLocked bool, lockCountry, ipCountry string) bool {
	return !ipLocked && lockCountry != "" && lockCountry != "00" && ipCountry != "" && !strings.EqualFold(lockCountry, ipCountry)
}

func loadIPLocationFile(path string) ([]ipLocationRange, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseIPLocationCSV(file)
}

func parseIPLocationCSV(source io.Reader) ([]ipLocationRange, error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = 4
	var ranges []ipLocationRange
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		from, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(row[0]), "\ufeff"), 10, 32)
		if err != nil {
			return nil, err
		}
		to, err := strconv.ParseUint(strings.TrimSpace(row[1]), 10, 32)
		if err != nil || from > to {
			return nil, errors.New("invalid IP location range")
		}
		ranges = append(ranges, ipLocationRange{from: uint32(from), to: uint32(to), country: strings.ToLower(strings.TrimSpace(row[2]))})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].from < ranges[j].from })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].from < ranges[i-1].to {
			return nil, errors.New("overlapping IP location ranges")
		}
	}
	return ranges, nil
}

func (s *Server) countryForIP(ip string) string {
	if s == nil || len(s.ipLocations) == 0 {
		return ""
	}
	address, err := netip.ParseAddr(ip)
	if err != nil || !address.Is4() {
		return ""
	}
	bytes := address.As4()
	value := binary.BigEndian.Uint32(bytes[:])
	index := sort.Search(len(s.ipLocations), func(index int) bool { return value < s.ipLocations[index].to })
	if index == len(s.ipLocations) || value < s.ipLocations[index].from {
		return ""
	}
	return s.ipLocations[index].country
}
