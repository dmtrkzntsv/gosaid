using System;
using System.Windows.Forms;

namespace GosaidUI;

internal static class Program
{
    [STAThread]
    private static void Main()
    {
        ApplicationConfiguration.Initialize();
        using var ctx = new TrayApplicationContext();
        Application.Run(ctx);
    }
}
