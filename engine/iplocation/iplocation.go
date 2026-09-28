package iplocation

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

type Range struct{ From, To uint32; Country string }

type Store struct{ ranges []Range }

func CountryLockMismatch(ipLocked bool, lockCountry, ipCountry string) bool {
	return !ipLocked && lockCountry != "" && lockCountry != "00" && ipCountry != "" && !strings.EqualFold(lockCountry, ipCountry)
}

func Load(path string) (*Store, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ParseCSV(file)
}

func ParseCSV(source io.Reader) (*Store, error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = 4
	var ranges []Range
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
		ranges = append(ranges, Range{From: uint32(from), To: uint32(to), Country: strings.ToLower(strings.TrimSpace(row[2]))})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].From < ranges[j].From })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].From < ranges[i-1].To {
			return nil, errors.New("overlapping IP location ranges")
		}
	}
	return &Store{ranges: ranges}, nil
}

func (s *Store) Country(ip string) string {
	if s == nil || len(s.ranges) == 0 {
		return ""
	}
	address, err := netip.ParseAddr(ip)
	if err != nil || !address.Is4() {
		return ""
	}
	bytes := address.As4()
	value := binary.BigEndian.Uint32(bytes[:])
	index := sort.Search(len(s.ranges), func(index int) bool { return value < s.ranges[index].To })
	if index == len(s.ranges) || value < s.ranges[index].From {
		return ""
	}
	return s.ranges[index].Country
}
