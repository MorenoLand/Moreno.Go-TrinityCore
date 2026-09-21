package world

import (
	"context"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type weatherType uint8

const (
	weatherFine weatherType = iota
	weatherRain
	weatherSnow
	weatherStorm
)

type zoneWeather struct {
	Zone      uint32
	Chances   [4][3]uint8
	Type      weatherType
	Intensity float32
	Next      time.Time
}

func (s *Server) weatherInterval() time.Duration {
	if s == nil || s.Config.WeatherChangeInterval == 0 {
		return 10 * time.Minute
	}
	return time.Duration(s.Config.WeatherChangeInterval) * time.Millisecond
}

func (w *zoneWeather) regenerate(now time.Time) bool {
	if w == nil {
		return false
	}
	u := rand.Intn(100)
	if u < 30 {
		return false
	}
	oldType, oldIntensity := w.Type, w.Intensity
	season := ((now.YearDay() - 1 - 78 + 365) / 91) % 4
	if u < 60 && w.Intensity < 0.33333334 {
		w.Type, w.Intensity = weatherFine, 0
	}
	if u < 60 && w.Type != weatherFine {
		w.Intensity -= 0.33333334
		return true
	}
	if u < 90 && w.Type != weatherFine {
		w.Intensity += 0.33333334
		return true
	}
	if w.Type != weatherFine {
		if w.Intensity < 0.33333334 {
			w.Intensity = 0.9999
			return true
		}
		if w.Intensity > 0.6666667 && rand.Intn(100) < 50 {
			w.Intensity -= 0.6666667
			return true
		}
		w.Type, w.Intensity = weatherFine, 0
	}
	chances := w.Chances[season]
	rnd := rand.Intn(100) + 1
	first, second, third := int(chances[0]), int(chances[0]+chances[1]), int(chances[0]+chances[1]+chances[2])
	switch {
	case rnd <= first:
		w.Type = weatherRain
	case rnd <= second:
		w.Type = weatherSnow
	case rnd <= third:
		w.Type = weatherStorm
	default:
		w.Type = weatherFine
	}
	if w.Type == weatherFine {
		w.Intensity = 0
	} else if u < 90 {
		w.Intensity = float32(rand.Float64()) * 0.3333
	} else if rand.Intn(100) < 50 {
		w.Intensity = float32(rand.Float64())*0.3333 + 0.3334
	} else {
		w.Intensity = float32(rand.Float64())*0.3333 + 0.6667
	}
	return w.Type != oldType || w.Intensity != oldIntensity
}

func (w *zoneWeather) packet() []byte {
	if w == nil {
		return nil
	}
	intensity := w.Intensity
	if intensity >= 1 {
		intensity = 0.9999
	}
	if intensity < 0 {
		intensity = 0.0001
	}
	state := uint32(0)
	if intensity >= 0.27 {
		switch w.Type {
		case weatherRain:
			if intensity < 0.40 {
				state = 3
			} else if intensity < 0.70 {
				state = 4
			} else {
				state = 5
			}
		case weatherSnow:
			if intensity < 0.40 {
				state = 6
			} else if intensity < 0.70 {
				state = 7
			} else {
				state = 8
			}
		case weatherStorm:
			if intensity < 0.40 {
				state = 22
			} else if intensity < 0.70 {
				state = 41
			} else {
				state = 42
			}
		}
	}
	return protocol.BuildWeather(state, intensity, false)
}

func (s *Server) ensureZoneWeather(ctx context.Context, zone uint32, sess *session) {
	if s == nil || !s.Config.WeatherEnabled || zone == 0 || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.weatherMu.Lock()
	weather := s.weather[zone]
	var packet []byte
	if weather != nil {
		packet = weather.packet()
	}
	s.weatherMu.Unlock()
	if weather == nil {
		var values [12]int64
		args := make([]any, len(values)+1)
		args[0] = zone
		for i := range values {
			args[i+1] = &values[i]
		}
		if err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT spring_rain_chance, spring_snow_chance, spring_storm_chance, summer_rain_chance, summer_snow_chance, summer_storm_chance, fall_rain_chance, fall_snow_chance, fall_storm_chance, winter_rain_chance, winter_snow_chance, winter_storm_chance FROM game_weather WHERE zone = ?`, args...).Scan(args[1:]...); err != nil {
			return
		}
		weather = &zoneWeather{Zone: zone, Next: time.Now().Add(s.weatherInterval())}
		for season := 0; season < 4; season++ {
			for kind := 0; kind < 3; kind++ {
				value := values[season*3+kind]
				if value < 0 {
					value = 0
				}
				if value > 100 {
					value = 25
				}
				weather.Chances[season][kind] = uint8(value)
			}
		}
		weather.regenerate(time.Now())
		s.weatherMu.Lock()
		if s.weather == nil {
			s.weather = make(map[uint32]*zoneWeather)
		}
		if existing := s.weather[zone]; existing != nil {
			weather = existing
		} else {
			s.weather[zone] = weather
		}
		packet = weather.packet()
		s.weatherMu.Unlock()
	}
	if sess != nil && packet != nil {
		_ = sess.write(uint16(protocol.OpcodeSMSG_WEATHER), packet, true)
	}
}

func (s *Server) updateWeather(ctx context.Context, now time.Time) {
	if s == nil || !s.Config.WeatherEnabled {
		return
	}
	type update struct {
		zone   uint32
		packet []byte
	}
	updates := make([]update, 0)
	s.weatherMu.Lock()
	for zone, weather := range s.weather {
		if weather == nil || now.Before(weather.Next) {
			continue
		}
		weather.Next = now.Add(s.weatherInterval())
		if weather.regenerate(now) {
			updates = append(updates, update{zone: zone, packet: weather.packet()})
		}
	}
	s.weatherMu.Unlock()
	if len(updates) == 0 {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.playerLoaded && sess.player != nil {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, update := range updates {
		for _, sess := range sessions {
			if sess.player.Zone == update.zone {
				_ = sess.write(uint16(protocol.OpcodeSMSG_WEATHER), update.packet, true)
			}
		}
	}
}
