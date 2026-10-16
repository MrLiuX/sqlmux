# Scene 3 (PostgreSQL, needs docker compose up): the SQL editor; completion with the enum values, SPC ll formats, the result below, gj for JSON.
# Not yet recorded: check the frames (the value completion after "status = ", the formatted SQL, the result pane).
Output renders/query.mp4
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
Ctrl+l
Sleep 300ms
Type "i"
Sleep 200ms
Type "select o.id, u.name, o.status, o.amount from t_order o join t_user u on u.id = o.user_id where o.status = "
Sleep 1500ms
Type "'done' order by o.amount desc"
Sleep 600ms
Escape
Sleep 100ms
Type " ll"
Sleep 1500ms
Enter
Wait+Screen /rows/
Sleep 1500ms
Ctrl+j
Sleep 400ms
Type "gj"
Sleep 2s
Escape
Sleep 100ms
Sleep 500ms
