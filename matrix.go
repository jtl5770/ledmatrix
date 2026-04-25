package main

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

const (
	MatrixWidth   = 9
	MatrixHeight  = 34
	MaxBrightness = 255
)

func checkCoords(x, y int) error {
	if x < 0 || x >= MatrixWidth || y < 0 || y >= MatrixHeight {
		return fmt.Errorf("coordinates (%d, %d) out of range. X must be 0-8 and Y must be 0-33", x, y)
	}
	return nil
}

func checkBrightness(brightness int) error {
	if brightness < 0 || brightness > 255 {
		return fmt.Errorf("brightness %d out of range. Brightness must be 0-255", brightness)
	}
	return nil
}

type Matrix struct {
	matrix            []byte
	defaultBrightness byte
	serialPort        serial.Port
}

func NewMatrix(defaultBrightness int, portName string) (*Matrix, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	p, err := serial.Open(portName, mode)
	if err != nil {
		return nil, err
	}

	// Set a short read timeout to prevent hanging if we ever use Read
	// and to ensure the driver is responsive.
	p.SetReadTimeout(100 * time.Millisecond)

	m := &Matrix{
		matrix:            make([]byte, 312),
		defaultBrightness: byte(defaultBrightness),
		serialPort:        p,
	}
	m.Reset(0)
	return m, nil
}

func (m *Matrix) Reopen(portName string) error {
	if m.serialPort != nil {
		m.serialPort.Close()
	}
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	p, err := serial.Open(portName, mode)
	if err != nil {
		return err
	}
	p.SetReadTimeout(100 * time.Millisecond)
	m.serialPort = p
	return nil
}

func (m *Matrix) Close() error {
	return m.serialPort.Close()
}

func (m *Matrix) Reset(brightness int) {
	for i := range m.matrix {
		m.matrix[i] = byte(brightness)
	}
}

func (m *Matrix) SetBrightness(brightness int) error {
	if err := checkBrightness(brightness); err != nil {
		return err
	}
	m.defaultBrightness = byte(brightness)
	_, err := m.Send(0x00, []byte{m.defaultBrightness}, false)
	return err
}

func (m *Matrix) SetMatrix(x, y int, brightness *int) error {
	b := int(m.defaultBrightness)
	if brightness != nil {
		b = *brightness
	}

	if err := checkCoords(x, y); err != nil {
		return err
	}
	if err := checkBrightness(b); err != nil {
		return err
	}

	m.matrix[(y*MatrixWidth)+x] = byte(b)
	return nil
}

func (m *Matrix) GetMatrix(x, y int) (int, error) {
	if err := checkCoords(x, y); err != nil {
		return 0, err
	}
	return int(m.matrix[(y*MatrixWidth)+x]), nil
}

func betterate(start, to int) []int {
	reverse := start > to
	if reverse {
		start, to = to, start
	}

	size := to - start + 1
	result := make([]int, size)
	for i := 0; i < size; i++ {
		if reverse {
			result[i] = to - i
		} else {
			result[i] = start + i
		}
	}
	return result
}

func (m *Matrix) DrawLine(p1, p2 []int, fade int, brightness *int) error {
	b := int(m.defaultBrightness)
	if brightness != nil {
		b = *brightness
	}

	var points [][]int
	if p1[0] == p2[0] {
		yPoints := betterate(p1[1], p2[1])
		points = make([][]int, len(yPoints))
		for i, y := range yPoints {
			points[i] = []int{p1[0], y}
		}
	} else if p1[1] == p2[1] {
		xPoints := betterate(p1[0], p2[0])
		points = make([][]int, len(xPoints))
		for i, x := range xPoints {
			points[i] = []int{x, p1[1]}
		}
	} else {
		return fmt.Errorf("coordinates %v %v form diagonal line\nDrawLine() only supports horizontal and vertical lines", p1, p2)
	}

	if fade > len(points) {
		return fmt.Errorf("fade length %d longer than line length %d", fade, len(points))
	}

	drawPoints := points
	if fade > 0 {
		drawPoints = points[:len(points)-fade]
	}

	for _, p := range drawPoints {
		if err := m.SetMatrix(p[0], p[1], &b); err != nil {
			return err
		}
	}

	if fade > 0 {
		bDiff := b / (fade + 1)
		for i := 0; i < fade; i++ {
			pIdx := len(points) - fade + i
			fadeBrightness := b - ((i + 1) * bDiff)
			if err := m.SetMatrix(points[pIdx][0], points[pIdx][1], &fadeBrightness); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *Matrix) Draw2D(image [][]int, x, y int, override bool) error {
	for yOff, row := range image {
		for xOff, value := range row {
			if !override && value == 0 {
				continue
			}
			v := value
			if err := m.SetMatrix(x+xOff, y+yOff, &v); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Matrix) Send(commandID byte, parameters []byte, withResponse bool) ([]byte, error) {
	if m.serialPort == nil {
		return nil, fmt.Errorf("serial port is not open")
	}
	data := append([]byte{0x32, 0xAC, commandID}, parameters...)
	_, err := m.serialPort.Write(data)
	if err != nil {
		return nil, err
	}

	if withResponse {
		buf := make([]byte, 32)
		n, err := m.serialPort.Read(buf)
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}
	return nil, nil
}

func (m *Matrix) CSend() error {
	for i := 0; i < MatrixWidth; i++ {
		var col []byte
		for y := i; y < 312; y += MatrixWidth {
			col = append(col, m.matrix[y])
		}
		_, err := m.Send(0x07, append([]byte{byte(i)}, col...), false)
		if err != nil {
			return err
		}
	}

	_, err := m.Send(0x08, []byte{}, false)
	return err
}

func (m *Matrix) QSend(brightness *int) error {
	b := int(m.defaultBrightness)
	if brightness != nil {
		b = *brightness
	}

	matrixEncoded := make([]byte, 0, 39) // 312 / 8 = 39

	for i := 0; i < 312; i += 8 {
		var line byte
		for j := 0; j < 8; j++ {
			if i+j < 312 && m.matrix[i+j] > 0 {
				// Python: line = [bool(i) for i in self._matrix[i:i + 8][::-1]]
				// line = int("".join(["01"[i] for i in line]), 2)
				// [::-1] means reverse.
				// So the first element in slice (i+0) becomes the LEAST significant bit?
				// line[7] is matrix[i+0], line[6] is matrix[i+1], ..., line[0] is matrix[i+7]
				// int("".join(...), 2) where join is line[0]line[1]...line[7]
				// So if matrix[i+0] is set, it's the 8th bit in the string, which is 2^0?
				// Let's re-verify.
				// Python: line = [True, False, ...] (size 8)
				// reverse: [..., False, True]
				// join: "01" string. If line[0] is True, it puts "1".
				// "10000000" reversed is "00000001" -> 1.
				// So matrix[i+0] corresponds to 2^0?
				// Let's see: line = [m[i+0], m[i+1], ..., m[i+7]]
				// reversed: [m[i+7], m[i+6], ..., m[i+0]]
				// bit string: bit(m[i+7])bit(m[i+6])...bit(m[i+0])
				// So m[i+0] is the 0-th bit (value 1).
				line |= (1 << j)
			}
		}
		matrixEncoded = append(matrixEncoded, line)
	}

	_, err := m.Send(0x06, matrixEncoded, false)
	if err != nil {
		return err
	}
	_, err = m.Send(0x00, []byte{byte(b)}, false)
	return err
}
