# Scene 1: a SQLite file; the tree with a filter, a table tab, WHERE, paging, the JSON view, a flash jump.
Output renders/sqlite.mp4
Set Shell zsh
Set FontFamily "Maple Mono NF CN"
Set FontSize 18
Set Width 1600
Set Height 900
Set Padding 12
Hide
Type `cd /tmp/sqlmux-demo && export PATH=$PWD/bin:$PATH XDG_CONFIG_HOME=$PWD/config XDG_DATA_HOME=$PWD/data XDG_STATE_HOME=$PWD/state PS1='$ ' && clear`
Enter
Type "sqlmux sqlite"
Enter
Wait+Screen /Tables/
Sleep 500ms
Show
Sleep 1s
Ctrl+h
Sleep 300ms
Type "/"
Sleep 300ms
Type "t_order"
Sleep 800ms
Enter
Sleep 400ms
Enter
Wait+Screen /LIMIT/
Sleep 1500ms
Type "/"
Sleep 400ms
Type "status = 'done'"
Sleep 800ms
Enter
Sleep 1500ms
Type ">"
Sleep 1s
Type "<"
Sleep 800ms
Type "gj"
Sleep 2s
Type "s"
Sleep 600ms
Type "am"
Sleep 2s
Escape
Sleep 100ms
Escape
Sleep 100ms
Sleep 1s
