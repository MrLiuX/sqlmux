# Scene 2 (PostgreSQL, needs docker compose up): edit cells inline, pick an enum value, save everything in one transaction.
# Not yet recorded: check the frames (the enum picker on the status cell, the Saved toast) before the cut.
Output renders/edit.mp4
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
Ctrl+h
Sleep 300ms
Type "/"
Type "t_order"
Sleep 600ms
Enter
Sleep 400ms
Enter
Wait+Screen /LIMIT/
Sleep 1200ms
Type "lll"
Sleep 400ms
Type "i"
Sleep 300ms
Ctrl+u
Type "99.50"
Sleep 500ms
Escape
Sleep 100ms
Type "jh"
Sleep 400ms
Enter
Sleep 1200ms
Type "j"
Sleep 500ms
Enter
Sleep 800ms
Ctrl+S
Wait+Screen /Saved/
Sleep 2s
