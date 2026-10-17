# Collage tile 8: Select rows, act on them
Output renders/shots/select.mp4
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
Type "jj"
Sleep 300ms
Show
Type "v"
Sleep 400ms
Type "jjj"
Sleep 2s
