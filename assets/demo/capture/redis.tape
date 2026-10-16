# Scene 5: Redis; namespaces in the tree, a hash as a table (edit + save), the console with the signature line and a reply as a table.
Output renders/redis.mp4
Set Shell zsh
Set FontFamily "Maple Mono NF CN"
Set FontSize 18
Set Width 1600
Set Height 900
Set Padding 12
Hide
Type `cd /tmp/sqlmux-demo && export PATH=$PWD/bin:$PATH XDG_CONFIG_HOME=$PWD/config XDG_DATA_HOME=$PWD/data XDG_STATE_HOME=$PWD/state PS1='$ ' && clear`
Enter
Type "sqlmux redis"
Enter
Wait+Screen /keys/
Sleep 500ms
Show
Sleep 1s
Ctrl+h
Sleep 300ms
Type "/"
Sleep 300ms
Type "hash:user"
Sleep 800ms
Enter
Sleep 400ms
Enter
Wait+Screen /TYPE hash/
Sleep 1500ms
Type "jjjl"
Sleep 400ms
Type "i"
Sleep 300ms
Ctrl+u
Type "Ann Lee"
Sleep 400ms
Escape
Sleep 100ms
Ctrl+S
Sleep 1500ms
Ctrl+l
Sleep 300ms
Ctrl+l
Sleep 300ms
Type "i"
Type "HGETALL hash:user"
Sleep 1200ms
Escape
Sleep 100ms
Enter
Wait+Screen /field/
Sleep 1500ms
Type "o"
Type "SET demo:greeting hello EX "
Sleep 2s
Escape
Sleep 100ms
Type "o"
Enter
Type "ZRANGE zset:rank 0 -1 WITHSCORES"
Escape
Sleep 100ms
Enter
Wait+Screen /member/
Sleep 2s
