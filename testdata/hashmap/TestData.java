import java.lang.ProcessHandle;
import java.io.File;
import java.util.HashMap;

class Object1 {
    HashMap<Integer, Integer> map = new HashMap<>();
}

public class TestData {
    private static Object1 o1 = new Object1();

    public static void main(String[] args) throws Exception {
        o1.map.put(1,2);
        o1.map.put(3,4);
        o1.map.put(5,6);

        String dumpFileName = args[0];
        if (new File(dumpFileName).delete()) {
            System.out.println("dump file removed: " + dumpFileName);
        }
        long pid = ProcessHandle.current().pid();
        Process exec = Runtime.getRuntime().exec(new String[]{"jcmd", "" + pid, "GC.heap_dump",
                dumpFileName});
        exec.waitFor();
        System.out.println("Exit code: " + exec.exitValue());
        if (new File(dumpFileName).exists()) {
            System.out.println("dump file generated: " + dumpFileName);
        } else {
            System.out.println("dump file failed: PID=" + pid);
        }
    }
}
