Require changes
Require diffnav
Set Shell "bash"
Set Theme "Catppuccin Macchiato"
Set Width 1440
Set Height 900
Set FontFamily "Liga SFMono Nerd Font"
Set FontSize 18
Set Framerate 20
Set TypingSpeed 45ms
Set WaitTimeout 60s
Hide
Type "export PS1='$ '; clear"
Enter
Show
Type "git show --stat --oneline HEAD"
Enter
Sleep 6s
Type "changes interactive --view commit --no-notes --no-symbols --no-calls --no-groups --quiet"
Enter
Wait+Screen /check-release.sh/
Sleep 4s
Ctrl+H
Type "jj"
Enter
Sleep 5s
Type "o"
Sleep 8s
Type "?"
Sleep 6s
Type "q"
Sleep 1s
Type "q"
Wait+Screen /changes .*detached/
Sleep 6s
Type "q"
Sleep 2s
