Require changes

Set Shell "zsh"
Set Theme "Catppuccin Macchiato"
Set FontSize 17
Set Width 1200
Set Height 680
Set Padding 20
Set Framerate 20
Set TypingSpeed 25ms

Sleep 1s

Type "changes note add --file main.go --line 8 --message 'Keep the fallback explicit' >/dev/null"
Enter
Sleep 2s

Type "changes"
Enter
Wait+Screen /line 8@right/
Sleep 3s
