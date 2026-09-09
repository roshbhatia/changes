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

Type "changes note add --provider local-notes --file internal/auth/token.go --line 6 --origin agent --author harness --session token-review --message 'Preserve the token prefix boundary' >/dev/null"
Enter
Sleep 2s

Type "clear; changes --color always"
Enter
Wait+Screen /Preserve the token prefix boundary/
Sleep 3s
