package com.buildworld.demo;

public final class App {
    private App() {}

    static String message(String name) {
        return "hello, " + name;
    }

    public static void main(String[] args) {
        if (args.length == 1 && "--self-test".equals(args[0])) {
            if (!"hello, matrix".equals(message("matrix"))) {
                throw new IllegalStateException("Java smoke test failed");
            }
            System.out.println("java smoke test passed");
            return;
        }
        System.out.println(message("buildworld"));
    }
}
