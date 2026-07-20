namespace BuildWorld.CSharp.Minimal;

internal static class Program
{
    private static string Message(string name) => $"hello, {name}";

    private static int Main()
    {
        if (Message("matrix") != "hello, matrix")
        {
            Console.Error.WriteLine("C# smoke test failed");
            return 1;
        }

        Console.WriteLine(Message("buildworld"));
        return 0;
    }
}
