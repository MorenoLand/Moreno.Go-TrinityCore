package world

import (
	"errors"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

type transportVector struct {
	x float32
	y float32
	z float32
}

type transportPathFrame struct {
	point            wotlk.TaxiSplinePoint
	teleport         bool
	orientation      float32
	curve            [4]transportVector
	distFromPrev     float32
	distSinceStop    float32
	distUntilStop    float32
	timeFrom         float32
	timeTo           float32
	arrival          uint32
	departure        uint32
	nextArrival      uint32
	nextDistFromPrev float32
}

type TransportTrajectory struct {
	frames       []transportPathFrame
	speed        float32
	acceleration float32
	accelTime    float32
	accelDist    float32
	period       uint32
}

func NewTransportTrajectory(points []wotlk.TaxiSplinePoint, speed, acceleration float32) (*TransportTrajectory, error) {
	if len(points) < 2 || speed <= 0 || acceleration <= 0 {
		return nil, errors.New("transport route requires at least two points and positive speed and acceleration")
	}
	orientations := transportInitialOrientations(points)
	frames := make([]transportPathFrame, 0, len(points))
	mapChange := false
	for index, point := range points {
		if mapChange {
			mapChange = false
			continue
		}
		if index+1 < len(points) && (point.Flags&1 != 0 || point.MapID != points[index+1].MapID) {
			if len(frames) == 0 {
				return nil, errors.New("transport route starts with a teleport marker")
			}
			frames[len(frames)-1].teleport = true
			mapChange = true
			continue
		}
		frames = append(frames, transportPathFrame{point: point, orientation: orientations[index]})
	}
	if len(frames) >= 2 {
		if !transportImportantFrame(frames[0].point) {
			frames = frames[1:]
		}
		if len(frames) > 0 && !transportImportantFrame(frames[len(frames)-1].point) {
			frames = frames[:len(frames)-1]
		}
	}
	if len(frames) < 2 {
		return nil, errors.New("transport route has fewer than two keyframes")
	}
	frames[len(frames)-1].teleport = true
	trajectory := &TransportTrajectory{frames: frames, speed: speed, acceleration: acceleration}
	trajectory.accelTime = trajectory.speed / trajectory.acceleration
	trajectory.accelDist = 0.5 * trajectory.speed * trajectory.speed / trajectory.acceleration
	trajectory.buildDistances()
	trajectory.buildTimes()
	if trajectory.period == 0 {
		return nil, errors.New("transport route has zero travel period")
	}
	return trajectory, nil
}

func (t *TransportTrajectory) Period() uint32 {
	if t == nil {
		return 0
	}
	return t.period
}

func (t *TransportTrajectory) Position(progress uint32) (uint32, float32, float32, float32, float32) {
	if t == nil || len(t.frames) == 0 || t.period == 0 {
		return 0, 0, 0, 0, 0
	}
	timer := progress % t.period
	index := 0
	for attempts := 0; attempts < len(t.frames)*2; attempts++ {
		frame := &t.frames[index]
		if timer >= frame.arrival {
			if frame.point.Flags == 2 && timer < frame.departure {
				return transportFramePosition(*frame)
			}
			if timer >= frame.departure && timer < frame.nextArrival {
				if frame.nextDistFromPrev <= 0 {
					return transportFramePosition(*frame)
				}
				now := float32(timer) * 0.001
				elapsed := now - float32(frame.departure)*0.001
				timeSinceStop := frame.timeFrom + elapsed
				timeUntilStop := frame.timeTo - elapsed
				var distance float32
				var segmentPosition float32
				if timeSinceStop < timeUntilStop {
					if timeSinceStop < t.accelTime {
						distance = 0.5 * t.acceleration * timeSinceStop * timeSinceStop
					} else {
						distance = t.accelDist + (timeSinceStop-t.accelTime)*t.speed
					}
					segmentPosition = distance - frame.distSinceStop
				} else {
					if timeUntilStop < t.accelTime {
						distance = 0.5 * t.acceleration * timeUntilStop * timeUntilStop
					} else {
						distance = t.accelDist + (timeUntilStop-t.accelTime)*t.speed
					}
					segmentPosition = frame.distUntilStop - distance
				}
				percent := segmentPosition / frame.nextDistFromPrev
				if percent < 0 {
					percent = 0
				} else if percent > 1 {
					percent = 1
				}
				position := transportCatmullRom(frame.curve[0], frame.curve[1], frame.curve[2], frame.curve[3], percent)
				derivative := transportCatmullRomDerivative(frame.curve[0], frame.curve[1], frame.curve[2], frame.curve[3], percent)
				orientation := float32(math.Mod(math.Atan2(float64(derivative.y), float64(derivative.x))+math.Pi, 2*math.Pi))
				return transportPointMap(frame.point), position.x, position.y, position.z, orientation
			}
		}
		index = (index + 1) % len(t.frames)
		if t.frames[index].teleport {
			index = (index + 1) % len(t.frames)
		}
	}
	return transportFramePosition(t.frames[0])
}

func transportFramePosition(frame transportPathFrame) (uint32, float32, float32, float32, float32) {
	return transportPointMap(frame.point), frame.point.X, frame.point.Y, frame.point.Z, float32(frame.orientation)
}

func transportPointMap(point wotlk.TaxiSplinePoint) uint32 {
	if point.MapID < 0 {
		return 0
	}
	return uint32(point.MapID)
}

func transportImportantFrame(point wotlk.TaxiSplinePoint) bool {
	return point.Flags == 2 || point.ArrivalEventID != 0 || point.DepartureEventID != 0
}

func transportInitialOrientations(points []wotlk.TaxiSplinePoint) []float32 {
	positions := make([]transportVector, 0, len(points)+3)
	first, second := transportPointVector(points[0]), transportPointVector(points[1])
	positions = append(positions, transportVectorAdd(first, transportVectorScale(transportVectorSub(first, second), 0.2)))
	for _, point := range points {
		positions = append(positions, transportPointVector(point))
	}
	last, previous := transportPointVector(points[len(points)-1]), transportPointVector(points[len(points)-2])
	positions = append(positions, transportVectorAdd(last, transportVectorScale(transportVectorSub(last, previous), 0.2)))
	positions = append(positions, transportVectorAdd(last, transportVectorSub(last, previous)))
	orientations := make([]float32, len(points))
	for index := range points {
		derivative := transportCatmullRomDerivative(positions[index], positions[index+1], positions[index+2], positions[index+3], 0)
		orientation := float32(math.Mod(math.Atan2(float64(derivative.y), float64(derivative.x))+math.Pi, 2*math.Pi))
		if orientation < 0 {
			orientation += float32(2 * math.Pi)
		}
		orientations[index] = orientation
	}
	return orientations
}

func (t *TransportTrajectory) buildDistances() {
	count := len(t.frames)
	edges := make([]float32, count)
	for index := range t.frames {
		next := (index + 1) % count
		currentFrame, nextFrame := t.frames[index], t.frames[next]
		if currentFrame.teleport || currentFrame.point.MapID != nextFrame.point.MapID {
			continue
		}
		p1, p2 := transportPointVector(currentFrame.point), transportPointVector(nextFrame.point)
		groupStart := index
		for groupStart > 0 && !t.frames[groupStart-1].teleport {
			groupStart--
		}
		groupEnd := count - 1
		for cursor := index; cursor < count; cursor++ {
			if t.frames[cursor].teleport {
				groupEnd = cursor
				break
			}
		}
		p0 := transportVectorSub(p1, transportVector{x: 1})
		if index > groupStart {
			p0 = transportPointVector(t.frames[index-1].point)
		}
		p3 := transportPointVector(t.frames[groupEnd].point)
		if next < groupEnd {
			p3 = transportPointVector(t.frames[next+1].point)
		}
		t.frames[index].curve = [4]transportVector{p0, p1, p2, p3}
		edges[index] = transportCatmullRomLength(p0, p1, p2, p3)
	}
	for index := range t.frames {
		t.frames[index].distFromPrev = edges[(index+count-1)%count]
		t.frames[index].nextDistFromPrev = edges[index]
	}
	firstStop, lastStop, hasStop := 0, 0, false
	for index := range t.frames {
		if t.frames[index].point.Flags == 2 {
			if !hasStop {
				firstStop = index
				hasStop = true
			}
			lastStop = index
		}
	}
	if !hasStop {
		firstStop, lastStop = 0, 0
	}
	distance := float32(0)
	for index := range t.frames {
		frameIndex := (index + lastStop) % count
		frame := &t.frames[frameIndex]
		if frame.point.Flags == 2 || frameIndex == lastStop {
			distance = 0
		} else {
			distance += frame.distFromPrev
		}
		frame.distSinceStop = distance
	}
	distance = 0
	for index := count - 1; index >= 0; index-- {
		frameIndex := (index + firstStop) % count
		frame := &t.frames[frameIndex]
		distance += t.frames[(frameIndex+1)%count].distFromPrev
		frame.distUntilStop = distance
		if frame.point.Flags == 2 || frameIndex == firstStop {
			distance = 0
		}
	}
}

func (t *TransportTrajectory) buildTimes() {
	for index := range t.frames {
		frame := &t.frames[index]
		totalDistance := frame.distSinceStop + frame.distUntilStop
		switch {
		case totalDistance < 2*t.accelDist:
			if frame.distSinceStop < frame.distUntilStop {
				frame.timeTo = 2*float32(math.Sqrt(float64(totalDistance/t.acceleration))) - float32(math.Sqrt(float64(2*frame.distSinceStop/t.acceleration)))
			} else {
				frame.timeTo = float32(math.Sqrt(float64(2 * frame.distUntilStop / t.acceleration)))
			}
		case frame.distSinceStop < t.accelDist:
			segmentTime := totalDistance/t.speed + t.speed/t.acceleration
			frame.timeTo = segmentTime - float32(math.Sqrt(float64(2*frame.distSinceStop/t.acceleration)))
		case frame.distUntilStop < t.accelDist:
			frame.timeTo = float32(math.Sqrt(float64(2 * frame.distUntilStop / t.acceleration)))
		default:
			frame.timeTo = frame.distUntilStop/t.speed + 0.5*t.speed/t.acceleration
		}
	}
	lastStop := 0
	for index := range t.frames {
		if t.frames[index].point.Flags == 2 {
			lastStop = index
		}
	}
	segmentTime := float32(0)
	for index := range t.frames {
		frameIndex := (index + lastStop) % len(t.frames)
		frame := &t.frames[frameIndex]
		if frame.point.Flags == 2 || frameIndex == lastStop {
			segmentTime = frame.timeTo
		}
		frame.timeFrom = segmentTime - frame.timeTo
	}
	t.frames[0].arrival = 0
	pathTime := float32(0)
	if t.frames[0].point.Flags == 2 {
		pathTime = float32(t.frames[0].point.Delay)
		t.frames[0].departure = uint32(pathTime * 1000)
	}
	for index := 1; index < len(t.frames); index++ {
		pathTime += t.frames[index-1].timeTo
		frame := &t.frames[index]
		if frame.point.Flags == 2 {
			frame.arrival = uint32(pathTime * 1000)
			t.frames[index-1].nextArrival = frame.arrival
			pathTime += float32(frame.point.Delay)
			frame.departure = uint32(pathTime * 1000)
		} else {
			pathTime -= frame.timeTo
			frame.arrival = uint32(pathTime * 1000)
			t.frames[index-1].nextArrival = frame.arrival
			frame.departure = frame.arrival
		}
	}
	t.frames[len(t.frames)-1].nextArrival = t.frames[len(t.frames)-1].departure
	t.period = t.frames[len(t.frames)-1].departure
}

func transportPointVector(point wotlk.TaxiSplinePoint) transportVector {
	return transportVector{x: point.X, y: point.Y, z: point.Z}
}

func transportVectorAdd(a, b transportVector) transportVector {
	return transportVector{x: a.x + b.x, y: a.y + b.y, z: a.z + b.z}
}

func transportVectorSub(a, b transportVector) transportVector {
	return transportVector{x: a.x - b.x, y: a.y - b.y, z: a.z - b.z}
}

func transportVectorScale(vector transportVector, scale float32) transportVector {
	return transportVector{x: vector.x * scale, y: vector.y * scale, z: vector.z * scale}
}

func transportCatmullRom(p0, p1, p2, p3 transportVector, t float32) transportVector {
	t2, t3 := t*t, t*t*t
	return transportVector{
		x: 0.5 * (2*p1.x + (-p0.x+p2.x)*t + (2*p0.x-5*p1.x+4*p2.x-p3.x)*t2 + (-p0.x+3*p1.x-3*p2.x+p3.x)*t3),
		y: 0.5 * (2*p1.y + (-p0.y+p2.y)*t + (2*p0.y-5*p1.y+4*p2.y-p3.y)*t2 + (-p0.y+3*p1.y-3*p2.y+p3.y)*t3),
		z: 0.5 * (2*p1.z + (-p0.z+p2.z)*t + (2*p0.z-5*p1.z+4*p2.z-p3.z)*t2 + (-p0.z+3*p1.z-3*p2.z+p3.z)*t3),
	}
}

func transportCatmullRomDerivative(p0, p1, p2, p3 transportVector, t float32) transportVector {
	t2 := t * t
	return transportVector{
		x: 0.5 * (-p0.x + p2.x + 2*(2*p0.x-5*p1.x+4*p2.x-p3.x)*t + 3*(-p0.x+3*p1.x-3*p2.x+p3.x)*t2),
		y: 0.5 * (-p0.y + p2.y + 2*(2*p0.y-5*p1.y+4*p2.y-p3.y)*t + 3*(-p0.y+3*p1.y-3*p2.y+p3.y)*t2),
		z: 0.5 * (-p0.z + p2.z + 2*(2*p0.z-5*p1.z+4*p2.z-p3.z)*t + 3*(-p0.z+3*p1.z-3*p2.z+p3.z)*t2),
	}
}

func transportCatmullRomLength(p0, p1, p2, p3 transportVector) float32 {
	previous, length := p1, float32(0)
	for sample := 1; sample <= 3; sample++ {
		position := transportCatmullRom(p0, p1, p2, p3, float32(sample)/3)
		delta := transportVectorSub(position, previous)
		length += float32(math.Sqrt(float64(delta.x*delta.x + delta.y*delta.y + delta.z*delta.z)))
		previous = position
	}
	return length
}
