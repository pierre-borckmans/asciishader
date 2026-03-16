package core

import (
	"os"

	"github.com/charmbracelet/x/termios"
)

// GetCellPixelSize returns the pixel dimensions of a single terminal cell.
// Returns (0, 0, err) if the terminal does not report pixel sizes.
func GetCellPixelSize() (cellW, cellH int, err error) {
	ws, err := termios.GetWinsize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, err
	}
	if ws.Xpixel == 0 || ws.Col == 0 || ws.Ypixel == 0 || ws.Row == 0 {
		return 0, 0, nil
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row), nil
}
