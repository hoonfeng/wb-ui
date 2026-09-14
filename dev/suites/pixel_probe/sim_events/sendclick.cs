using System;
using System.Runtime.InteropServices;
using System.Threading;

class SendClick
{
    [DllImport("user32.dll")]
    static extern IntPtr FindWindow(string lpClassName, string lpWindowName);

    [DllImport("user32.dll")]
    static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);

    [DllImport("user32.dll")]
    static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    static extern uint SendInput(uint nInputs, INPUT[] pInputs, int cbSize);

    [StructLayout(LayoutKind.Sequential)]
    struct RECT { public int left, top, right, bottom; }

    [StructLayout(LayoutKind.Sequential)]
    struct INPUT
    {
        public int type;
        public MOUSEINPUT mi;
    }

    [StructLayout(LayoutKind.Sequential)]
    struct MOUSEINPUT
    {
        public int dx;
        public int dy;
        public uint mouseData;
        public uint dwFlags;
        public uint time;
        public IntPtr dwExtraInfo;
    }

    const int INPUT_MOUSE = 0;
    const uint MOUSEEVENTF_MOVE = 0x0001;
    const uint MOUSEEVENTF_LEFTDOWN = 0x0002;
    const uint MOUSEEVENTF_LEFTUP = 0x0004;
    const uint MOUSEEVENTF_ABSOLUTE = 0x8000;

    static void Main(string[] args)
    {
        if (args.Length < 2) { Console.Error.WriteLine("Usage: SendClick <x> <y>"); return; }
        int targetX = int.Parse(args[0]);
        int targetY = int.Parse(args[1]);

        // Find wb-ui window
        IntPtr hwnd = FindWindow(null, "wb-ui: dev/suites/pixel_probe/test_html/step17_form.html");
        if (hwnd == IntPtr.Zero)
        {
            // Try partial match
            hwnd = FindWindowByTitleContains("step17");
        }
        if (hwnd == IntPtr.Zero) { Console.Error.WriteLine("Window not found"); return; }

        SetForegroundWindow(hwnd);
        Thread.Sleep(200);

        RECT rect;
        GetWindowRect(hwnd, out rect);
        int winW = rect.right - rect.left;
        int winH = rect.bottom - rect.top;

        // Convert client coords to screen absolute (0-65535)
        int absX = (rect.left + targetX) * 65535 / GetSystemMetrics(0);
        int absY = (rect.top + targetY) * 65535 / GetSystemMetrics(1);

        Console.WriteLine($"Window: ({rect.left},{rect.top})-({rect.right},{rect.bottom})");
        Console.WriteLine($"Screen: {GetSystemMetrics(0)}x{GetSystemMetrics(1)}");
        Console.WriteLine($"Clicking at screen abs=({absX},{absY}) client=({targetX},{targetY})");

        // Move mouse
        INPUT move = new INPUT();
        move.type = INPUT_MOUSE;
        move.mi.dx = absX;
        move.mi.dy = absY;
        move.mi.dwFlags = MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE;
        SendInput(1, new INPUT[] { move }, Marshal.SizeOf(typeof(INPUT)));
        Thread.Sleep(100);

        // Click
        INPUT down = new INPUT();
        down.type = INPUT_MOUSE;
        down.mi.dx = 0;
        down.mi.dy = 0;
        down.mi.dwFlags = MOUSEEVENTF_LEFTDOWN;
        SendInput(1, new INPUT[] { down }, Marshal.SizeOf(typeof(INPUT)));
        Thread.Sleep(50);

        INPUT up = new INPUT();
        up.type = INPUT_MOUSE;
        up.mi.dx = 0;
        up.mi.dy = 0;
        up.mi.dwFlags = MOUSEEVENTF_LEFTUP;
        SendInput(1, new INPUT[] { up }, Marshal.SizeOf(typeof(INPUT)));
        Thread.Sleep(50);

        Console.WriteLine("Click sent");
    }

    static IntPtr FindWindowByTitleContains(string partial)
    {
        EnumWindows((hwnd, lParam) =>
        {
            System.Text.StringBuilder sb = new System.Text.StringBuilder(256);
            GetWindowText(hwnd, sb, 256);
            if (sb.ToString().IndexOf(partial, StringComparison.OrdinalIgnoreCase) >= 0)
            {
                GCHandle handle = GCHandle.FromIntPtr(lParam);
                handle.Target = hwnd;
                return false;
            }
            return true;
        }, IntPtr.Zero);
        return IntPtr.Zero;
    }

    [DllImport("user32.dll")]
    static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);
    delegate bool EnumWindowsProc(IntPtr hwnd, IntPtr lParam);

    [DllImport("user32.dll", CharSet = CharSet.Auto)]
    static extern int GetWindowText(IntPtr hWnd, System.Text.StringBuilder text, int nMaxCount);

    [DllImport("user32.dll")]
    static extern int GetSystemMetrics(int nIndex);
}
