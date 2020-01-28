import java.lang.ProcessHandle;
import java.io.File;

class Recursion1 {
    Recursion2 r2;
}

class Recursion2 {
    Recursion1 r1;
}

public class TestData {
    private static Recursion1 r1 = new Recursion1();

    public static void main(String[] args) throws Exception {
        r1.r2 = new Recursion2();
        r1.r2.r1 = r1;

        String dumpFileName = args[0];
        if (new File(dumpFileName).delete()) {
            System.out.println("dump file removed: " + dumpFileName);
        }
        long pid = ProcessHandle.current().pid();
        Process exec = Runtime.getRuntime().exec(new String[] { "jcmd", "" + pid, "GC.heap_dump",
           dumpFileName });
        exec.waitFor();
        System.out.println("Exit code: " + exec.exitValue());
        if (new File(dumpFileName).exists()) {
            System.out.println("dump file generated: " + dumpFileName);
        } else {
            System.out.println("dump file failed: PID="+ pid);
        }
    }
}
