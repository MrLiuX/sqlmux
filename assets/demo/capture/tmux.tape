# Scene 4: tmux inside — the palette opens a table, ; runs quick SQL; one pane, then a split, then the result pane below; a new window; a new session.
Output renders/tmux.mp4
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
Ctrl+l
Sleep 200ms
Type " x"
Sleep 600ms
Show
Sleep 1s
Ctrl+p
Sleep 600ms
Type "@order"
Sleep 1200ms
Enter
Wait+Screen /LIMIT/
Sleep 1200ms
Type ";"
Sleep 500ms
Type "select count(*) from t_user"
Sleep 800ms
Enter
Sleep 300ms
Enter
Sleep 2500ms
Escape
Sleep 100ms
Sleep 300ms
Type " %"
Sleep 1000ms
Type "c"
Sleep 800ms
Type "i"
Type "select status, count(*) n from t_order group by 1"
Sleep 600ms
Escape
Sleep 100ms
Type "0"
Sleep 400ms
Enter
Wait+Screen /query_1 #1/
Sleep 2s
Type " c"
Sleep 1500ms
Type " n"
Sleep 1500ms
Ctrl+h
Sleep 300ms
Ctrl+p
Sleep 500ms
Type ">new session"
Sleep 800ms
Enter
Sleep 600ms
Ctrl+u
Type "reports"
Sleep 400ms
Enter
Sleep 1500ms
Type " )"
Sleep 1500ms
