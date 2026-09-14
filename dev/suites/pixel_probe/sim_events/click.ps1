Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Write-Host "=== Simulating clicks on wb-ui window ==="

# Find the window
$hwnd = @()
$titlePart = "step17_form"
$shell = New-Object -ComObject Shell.Application
$shell.Windows() | ForEach-Object {
    if ($_.Document.Title -like "*$titlePart*") { $hwnd = $_.HWND }
}

# If not found via Shell, use FindWindow
if (-not $hwnd) {
    $code = @'
[DllImport("user32.dll")] public static extern int FindWindow(string c, string w);
[DllImport("user32.dll")] public static extern bool SetForegroundWindow(int h);
[DllImport("user32.dll")] public static extern bool GetWindowRect(int h, out int l, out int t, out int r, out int b);
'@ -as [type]
    # Try simpler approach
}

Write-Host "Using mouse_event approach..."
# Get the wb-ui window using Windows API directly
Add-Type -AssemblyName Microsoft.VisualBasic

# Use .NET's built-in Cursor.Position for mouse control
$found = $false
$titleMatch = "step17_form"
$allProc = [System.Diagnostics.Process]::GetProcesses()
foreach ($p in $allProc) {
    if ($p.MainWindowTitle -like "*$titleMatch*" -or $p.MainWindowTitle -like "*wb-ui*") {
        Write-Host "Found window: $($p.MainWindowTitle) HWND=$($p.MainWindowHandle)"
        $found = $true
        [Microsoft.VisualBasic.Interaction]::AppActivate($p.Id)
        Start-Sleep -Milliseconds 300
        
        $rect = New-Object System.Drawing.Rectangle
        # Use GetWindowRect via P/Invoke
        Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] public static extern uint SendInput(uint n, INPUT[] p, int s);
    
    public struct RECT { public int left, top, right, bottom; }
    public struct MOUSEINPUT { public int dx, dy; public uint mouseData, dwFlags, time; public IntPtr dwExtraInfo; }
    public struct INPUT { public int type; public MOUSEINPUT mi; }
    
    const int INPUT_MOUSE = 0;
    const uint ABSOLUTE = 0x8000, MOVE = 0x0001, DOWN = 0x0002, UP = 0x0004;
    
    public static void ClickAt(IntPtr hwnd, int cx, int cy) {
        RECT r;
        GetWindowRect(hwnd, out r);
        int sw = GetSystemMetrics(0);
        int sh = GetSystemMetrics(1);
        int ax = (r.left + cx) * 65535 / sw;
        int ay = (r.top + cy) * 65535 / sh;
        Console.WriteLine("  rect=(" + r.left + "," + r.top + ")-(" + r.right + "," + r.bottom + ")");
        Console.WriteLine("  screen=(" + sw + "x" + sh + ") abs=(" + ax + "," + ay + ")");
        
        int sz = System.Runtime.InteropServices.Marshal.SizeOf(typeof(INPUT));
        
        SendInput(1, new INPUT[] { new INPUT { type=INPUT_MOUSE, mi=new MOUSEINPUT { dx=ax, dy=ay, dwFlags=MOVE|ABSOLUTE } } }, sz);
        System.Threading.Thread.Sleep(100);
        SendInput(1, new INPUT[] { new INPUT { type=INPUT_MOUSE, mi=new MOUSEINPUT { dwFlags=DOWN } } }, sz);
        System.Threading.Thread.Sleep(50);
        SendInput(1, new INPUT[] { new INPUT { type=INPUT_MOUSE, mi=new MOUSEINPUT { dwFlags=UP } } }, sz);
        Console.WriteLine("  click done");
    }
    
    [DllImport("user32.dll")] public static extern int GetSystemMetrics(int n);
}
"@

        # Click Username (80,140)
        Write-Host "[1] Click Username at (80,140)"
        [Win32]::ClickAt($p.MainWindowHandle, 80, 140)
        Start-Sleep -Milliseconds 500
        
        # Click Textarea (80,370)  
        Write-Host "[2] Click Textarea at (80,370)"
        [Win32]::ClickAt($p.MainWindowHandle, 80, 370)
        Start-Sleep -Milliseconds 500
        
        # Click Checkbox Option A (60,609)
        Write-Host "[3] Click Checkbox Option A at (60,609)"
        [Win32]::ClickAt($p.MainWindowHandle, 60, 609)
        Start-Sleep -Milliseconds 500
        
        # Click Radio Option 1 (60,772)
        Write-Host "[4] Click Radio Option 1 at (60,772)"
        [Win32]::ClickAt($p.MainWindowHandle, 60, 772)
        
        Write-Host "`nAll clicks sent!"
        break
    }
}

if (-not $found) {
    Write-Host "ERROR: wb-ui window not found. Looking..."
    Get-Process | Where-Object { $_.MainWindowTitle } | Select-Object MainWindowTitle | Format-Table
}
