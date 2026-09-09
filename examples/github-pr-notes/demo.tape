Require changes
Require gh

Set Shell "zsh"
Set Theme "Catppuccin Macchiato"
Set FontSize 17
Set Width 1200
Set Height 680
Set Padding 20
Set Framerate 20
Set TypingSpeed 25ms

Sleep 1s

Type "changes --color always HEAD^ HEAD"
Enter
Wait+Screen /Require the Bearer prefix/
Sleep 3s

Type "changes note list --from HEAD^ --to HEAD --provider github-pr"
Enter
Wait+Screen /reviewer via github-pr/
Sleep 3s
