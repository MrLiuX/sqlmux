# Scene 4 (PostgreSQL, needs docker compose up): the palette (@ finds a table on any connection), quick SQL with ;, tmux-like splits and windows.
# Not yet recorded: check the frames (the palette list, the quick SQL result, the split, the window line in the status bar).
Output renders/palette.mp4
Set Shell zsh
Set FontFamily "Maple Mono NF CN"
Set FontSize 18
Set Width 1600
Set Height 900
Set Padding 12
Hide
Type `cd /tmp/sqlmux-demo && export PATH=$PWD/bin:$PATH XDG_CONFIG_HOME=$PWD/config XDG_DATA_HOME=$PWD/data XDG_STATE_HOME=$PWD/state PS1='$ ' && clear`
Enter
Type "sqlmux play"
Enter
Wait+Screen /Tables/
Sleep 500ms
Show
Sleep 1s
Ctrl+p
Sleep 500ms
Type "@user"
Sleep 1200ms
Enter
Wait+Screen /LIMIT/
Sleep 1200ms
Type ";"
Sleep 400ms
Type "select status, count(*) from t_order group by 1"
Sleep 600ms
Enter
Wait+Screen /count/
Sleep 1500ms
Type " %"
Sleep 800ms
Ctrl+p
Sleep 300ms
Type "@order"
Sleep 800ms
Enter
Wait+Screen /t_order/
Sleep 1200ms
Type " c"
Sleep 1200ms
Type " n"
Sleep 1500ms
