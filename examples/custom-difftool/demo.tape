Require changes
Require delta
Require difft

Set Shell "zsh"
Set Theme "Catppuccin Macchiato"
Set FontSize 17
Set Width 1200
Set Height 680
Set Padding 20
Set Framerate 20
Set TypingSpeed 25ms

Sleep 1s

Type "changes --color always"
Enter
Wait+Screen /ValidToken/
Sleep 3s

Type "changes difftool .demo/before.go .demo/after.go internal/auth/token.go"
Enter
Wait+Screen /HasPrefix/
Sleep 3s
