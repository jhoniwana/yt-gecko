package tui

import "strings"

// snakeLogo is the app splash shown on the welcome tour page. It is a framed
// ASCII snake kept in sync with assets/logo-snake.txt.
var snakeLogo = strings.Join([]string{
	"_____________________________",
	"|                  _        |",
	"|                 /\"\\       |",
	"|                /o o\\      |",
	"|           _\\/  \\   / \\/_  |",
	"|            \\\\._/  /_.//   |",
	"|            `--,  ,----'   |",
	"|              /   /        |",
	"|    ^        /    \\        |",
	"|   /|       (      )       |",
	"|  / |     ,__\\    /__,     |",
	"|  \\ \\   _//---,  ,--\\\\_    |",
	"|   \\ \\   /\\  /  /   /\\     |",
	"|    \\ \\.___,/  /           |",
	"|     \\.______,/            |",
	"|                           |",
	"~~~~~~~~~~~~~~~~~~~~~~~~~~~~~",
}, "\n")
