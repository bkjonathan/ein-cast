// Package loginitem makes EinCast open when the person signs in. It implements
// app.LoginItem once per platform: a LaunchAgent on macOS, a Run key on Windows.
// Both start the app with BackgroundFlag so it comes up without a window in the
// way.
package loginitem

// BackgroundFlag is the command-line argument a login launch carries. main
// reads it to start with the window hidden.
const BackgroundFlag = "--background"
