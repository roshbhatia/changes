Require changes

Set Shell "zsh"
Set Theme "Catppuccin Macchiato"
Set FontSize 17
Set Width 1200
Set Height 680
Set Padding 20
Set Framerate 20
Set TypingSpeed 20ms

Sleep 1s

Type "nvim +6 internal/auth/token.go"
Enter
Wait+Screen /ValidToken/
Sleep 1s

Type ":ChangesNote"
Enter
Wait+Screen /Diff note:/
Type "Explain the token prefix boundary"
Enter
Wait+Screen /changes note:/
Sleep 2s

Type ":q"
Enter
Sleep 1s

Type "changes"
Enter
Wait+Screen /Explain the token prefix boundary/
Sleep 3s
