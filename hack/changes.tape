Require changes

Require git
Set Shell "bash"
Set Theme "Catppuccin Macchiato"
Set FontSize 18
Set Width 1440
Set Height 900
Set Padding 20
Set Framerate 20
Set TypingSpeed 65ms
Set WaitTimeout 60s

Hide
Type "export PS1='$ '; clear"
Enter
Show
Type "git show --stat --oneline HEAD"
Enter
Wait+Screen /validate core and provider release archives/
Sleep 8s
Type "changes interactive --view commit"
Enter
Wait+Screen /check-release.sh/
Sleep 7s
Ctrl+H
Type "jj"
Enter
Wait+Screen /archives/
Type "d"
Sleep 10s
Ctrl+K
Ctrl+D
Sleep 10s
Ctrl+U
Sleep 10s
Type "q"
Sleep 4s
