import java.lang.ProcessHandle;
import java.io.File;  

class Baz {
    int baaaaz = 3920;
}

class Bar {
    int gokurosan = 5963;
}

class Empty {
}

class Foo extends Bar{
    int a = 3;
    int b = 5;
    short c = 4189;
    String msg = "Hello";
    String nullField = null;
    Baz baz = new Baz();
    Empty empty = new Empty();
}

public class Hello {
    static Foo foo = new Foo();

    public static void main(String[] args) throws Exception {
        if (args.length == 1) {
            System.out.println("Usage: java Hello file.hprof");
        }
        String dumpFileName = "/tmp/heapdump.hprof";
        if (new File(dumpFileName).delete()) {
            System.out.println("dump file removed.");
        }
        long pid = ProcessHandle.current().pid();
        Process exec = Runtime.getRuntime().exec(new String[] { "jcmd", "" + pid, "GC.heap_dump",
           dumpFileName });
        exec.waitFor();
        System.out.println(exec.exitValue());
        if (new File(dumpFileName).exists()) {
            System.out.println("dump file generated.");
        } else {
            System.out.println("dump file failed: PID="+ pid);
        }
    }
}
