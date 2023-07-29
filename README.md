# FoLD

```
go install github.com/coreyog/fld
```

Code folding in the terminal.

Controls  
&nbsp;&nbsp;&nbsp;&nbsp;Up, Down: Move up and down the file.  
&nbsp;&nbsp;&nbsp;&nbsp;Space: Fold the section the cursor is in  
&nbsp;&nbsp;&nbsp;&nbsp;Left, Right: Scroll left and right for long strings  
&nbsp;&nbsp;&nbsp;&nbsp;F: Fold all sections  
&nbsp;&nbsp;&nbsp;&nbsp;U: Unfold all sections  
&nbsp;&nbsp;&nbsp;&nbsp;Q, Esc: Quit  

It loads the entire file into memory, so it's not suitable for LARGE files.

Couldn't call it `fold` because that's already a linux tool. It cuts off lines at a
max length.
