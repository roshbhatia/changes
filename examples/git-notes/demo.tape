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

Type "changes note add --commit HEAD --provider git-notes --file internal/auth/token.go --line 6 --origin agent --author review-agent --message 'Reject empty tokens before session lookup'"
Enter
Sleep 2s

Type "git push origin refs/notes/changes"
Enter
Sleep 2s

Type "cd ../reviewer"
Enter
Type "git fetch origin refs/notes/changes:refs/notes/changes-incoming"
Enter
Sleep 2s
Type "git notes --ref=refs/notes/changes merge -s cat_sort_uniq refs/notes/changes-incoming"
Enter
Sleep 2s

Type "changes HEAD^ HEAD"
Enter
Wait+Screen /Reject empty tokens before session lookup/
Sleep 3s
