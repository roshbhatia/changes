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

Type "echo 'Offline replay: recorded Codex review, local parser fixture'"
Enter
Sleep 2s

Type "changes note generate --provider codex-review --store local-notes >/dev/null"
Enter
Sleep 2s

Type "changes --color always"
Enter
Wait+Screen /Require the Bearer prefix/
Sleep 3s
