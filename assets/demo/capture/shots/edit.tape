# Collage tile 7: Edit a cell, save as one transaction
Output renders/shots/edit.mp4
Set Shell zsh
Set FontFamily "Maple Mono NF CN"
Set FontSize 20
Set Width 1200
Set Height 676
Set Padding 12
Hide
Type `cd /tmp/sqlmux-demo && export PATH=$PWD/bin:$PATH XDG_CONFIG_HOME=$PWD/config XDG_DATA_HOME=$PWD/data XDG_STATE_HOME=$PWD/state PS1='$ ' && clear`
Enter
Type "sqlmux sqlite"
Enter
Wait+Screen /Tables/
Sleep 500ms
Ctrl+l
Sleep 200ms
Type " x"
Sleep 600ms
Type " b"
Sleep 600ms
Ctrl+p
Sleep 600ms
Type "@order"
Sleep 1200ms
Enter
Wait+Screen /LIMIT/
Sleep 800ms
Type "lll"
Sleep 300ms
Type "i"
Sleep 300ms
Ctrl+u
Type "1999.5"
Escape
Sleep 300ms
Type "jll"
Sleep 300ms
Show
Type "e"
Sleep 800ms
Type "i"
Sleep 300ms
Type "gift wrap, leave at the door"
Escape
Sleep 2s
