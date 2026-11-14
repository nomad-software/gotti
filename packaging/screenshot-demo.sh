#!/bin/bash
# Content for the README screenshot (see packaging/screenshot.sh).
#   screenshot-demo.sh show   colours, styles, Unicode and box drawing
#   screenshot-demo.sh ls     a coloured directory listing
# Either way it ends in an interactive shell with a short prompt.
export PS1='\[\e[32m\]~/termite\[\e[0m\] \$ '
sleep 1 # let the window reach its final size, so output fits the pane
case "${1:-show}" in
show)
	printf '\e[1mTermite\e[0m  \e[2mGPU-rendered terminal\e[0m\n\n'
	printf ' \e[1mbold\e[0m \e[3mitalic\e[0m \e[4munderline\e[0m \e[4:3;58:2::220:50:47mcurly\e[0m \e[9mstrike\e[0m \e[7minverse\e[0m\n\n'
	for i in $(seq 0 15); do printf '\e[48;5;%dm   ' "$i"; done; printf '\e[0m\n'
	for i in $(seq 0 5 239); do printf '\e[48;2;%d;%d;%dm ' $((i)) $((120 + i / 3)) $((240 - i)); done; printf '\e[0m\n\n'
	printf ' ┌───────────┬──────────┐  ╭────────╮\n'
	printf ' │ wide 世界 │ emoji 🚀 │  │ ⣿⣿ ▓▒░ │\n'
	printf ' └───────────┴──────────┘  ╰────────╯\n\n'
	printf ' \e]8;;https://github.com/nomad-software/qute\e\\a hyperlink\e]8;;\e\\  and  https://go.dev\n\n'
	;;
ls)
	ls --color=always -F
	echo
	;;
esac
exec bash --norc
