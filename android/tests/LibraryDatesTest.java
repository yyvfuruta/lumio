import io.github.yyvfuruta.lumio.LibraryDates;
import java.time.ZoneId;
import java.time.Instant;
import java.time.Duration;

public final class LibraryDatesTest {
    public static void main(String[] args) {
        String[] leap = LibraryDates.range("2024-02", ZoneId.of("UTC"));
        if (!leap[0].equals("2024-02-01T00:00:00Z") || !leap[1].equals("2024-03-01T00:00:00Z")) throw new AssertionError("month bounds");
        if (Duration.between(Instant.parse(leap[0]), Instant.parse(leap[1])).toDays() != 29) throw new AssertionError("leap month");
        String[] dst = LibraryDates.range("2018-11-04", ZoneId.of("America/Sao_Paulo"));
        if (Duration.between(Instant.parse(dst[0]), Instant.parse(dst[1])).toHours() != 23) throw new AssertionError("local midnight during DST");
        if (!LibraryDates.day("2026-11-18T00:30:00Z", ZoneId.of("America/Sao_Paulo")).equals("2026-11-17")) throw new AssertionError("upload local date");
        if (!LibraryDates.day("2026-11-18T13:45:48-03:00", ZoneId.of("UTC")).equals("2026-11-18")) throw new AssertionError("offset timestamp");
        String[] year = LibraryDates.range("2026", ZoneId.of("UTC"));
        if (!year[0].equals("2026-01-01T00:00:00Z") || !year[1].equals("2027-01-01T00:00:00Z")) throw new AssertionError("year bounds");
        try { LibraryDates.range("2026-13", ZoneId.of("UTC")); throw new AssertionError("invalid month accepted"); } catch (java.time.DateTimeException expected) { }
        System.out.println("Android date/timeline tests passed");
    }
}
