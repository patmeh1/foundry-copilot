// Tiny helpers for screenshot driving.
//
// Usage:
//   swift /tmp/code_focus.swift activate <pid>      # bring app with this PID frontmost
//   swift /tmp/code_focus.swift list                # list visible Code windows: id pid title
//   swift /tmp/code_focus.swift winid <pid>         # print the visible window id for this PID
import Cocoa
import CoreGraphics

let args = CommandLine.arguments
guard args.count >= 2 else {
  print("usage: \(args[0]) activate <pid> | list | winid <pid>")
  exit(2)
}

switch args[1] {
case "activate":
  guard args.count == 3, let pid = pid_t(args[2]) else { exit(2) }
  guard let app = NSRunningApplication(processIdentifier: pid) else {
    FileHandle.standardError.write("no app for pid \(pid)\n".data(using: .utf8)!)
    exit(1)
  }
  let ok = app.activate(options: [.activateIgnoringOtherApps])
  print(ok ? "ok" : "failed")
case "list":
  let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
  guard let windows = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: AnyObject]] else { exit(1) }
  for w in windows {
    let owner = w[kCGWindowOwnerName as String] as? String ?? ""
    let layer = w[kCGWindowLayer as String] as? Int ?? -1
    guard owner == "Code", layer == 0 else { continue }
    let id = w[kCGWindowNumber as String] as? Int ?? 0
    let pid = w[kCGWindowOwnerPID as String] as? Int ?? 0
    let title = w[kCGWindowName as String] as? String ?? ""
    print("\(id) \(pid) \(title)")
  }
case "winid":
  guard args.count == 3, let want = Int(args[2]) else { exit(2) }
  let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
  guard let windows = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: AnyObject]] else { exit(1) }
  for w in windows {
    let owner = w[kCGWindowOwnerName as String] as? String ?? ""
    let layer = w[kCGWindowLayer as String] as? Int ?? -1
    let pid = w[kCGWindowOwnerPID as String] as? Int ?? -1
    let height = (w[kCGWindowBounds as String] as? [String: CGFloat])?["Height"] ?? 0
    // Pick a real main window (layer 0, height > 200) belonging to the wanted PID
    if owner == "Code" && layer == 0 && pid == want && height > 200 {
      let id = w[kCGWindowNumber as String] as? Int ?? 0
      print(id)
      exit(0)
    }
  }
  exit(1)
case "bounds":
  // Print "x y w h" (points, screen coords) of the isolated VS Code window.
  guard args.count == 3, let want = Int(args[2]) else { exit(2) }
  let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
  guard let windows = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: AnyObject]] else { exit(1) }
  for w in windows {
    let owner = w[kCGWindowOwnerName as String] as? String ?? ""
    let layer = w[kCGWindowLayer as String] as? Int ?? -1
    let pid = w[kCGWindowOwnerPID as String] as? Int ?? -1
    let bounds = w[kCGWindowBounds as String] as? [String: CGFloat] ?? [:]
    let height = bounds["Height"] ?? 0
    if owner == "Code" && layer == 0 && pid == want && height > 200 {
      let x = bounds["X"] ?? 0
      let y = bounds["Y"] ?? 0
      let h = bounds["Height"] ?? 0
      let wWidth = bounds["Width"] ?? 0
      print("\(Int(x)) \(Int(y)) \(Int(wWidth)) \(Int(h))")
      exit(0)
    }
  }
  exit(1)
default:
  print("unknown command \(args[1])")
  exit(2)
}
