import java.lang.ProcessHandle;
import java.io.File;

class Object1 {
    String stringEntry = "abcdefghijklmnopqrstuvwxyz";
}

public class TestData {
    private static Object1 r1 = new Object1();

    public static void main(String[] args) throws Exception {
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
