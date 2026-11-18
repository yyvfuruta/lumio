package io.github.yyvfuruta.lumio;

import java.time.Instant;
import java.time.LocalDate;
import java.time.Year;
import java.time.YearMonth;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.Locale;

/** Calendar boundaries shared by the native timeline, groups and selections. */
public final class LibraryDates {
    private LibraryDates() { }
    public static String[] range(String period, ZoneId zone) {
        if (period == null || period.isEmpty()) return new String[]{"", ""};
        LocalDate from, until;
        if (period.length() == 4) { from = Year.parse(period).atDay(1); until = from.plusYears(1); }
        else if (period.length() == 7) { from = YearMonth.parse(period).atDay(1); until = from.plusMonths(1); }
        else { from = LocalDate.parse(period); until = from.plusDays(1); }
        return new String[]{from.atStartOfDay(zone).toInstant().toString(), until.atStartOfDay(zone).toInstant().toString()};
    }
    public static String day(String timestamp, ZoneId zone) { return java.time.OffsetDateTime.parse(timestamp).atZoneSameInstant(zone).toLocalDate().toString(); }
    public static String label(String day, ZoneId zone) {
        LocalDate date = LocalDate.parse(day), today = LocalDate.now(zone);
        if (date.equals(today)) return "Hoje";
        if (date.equals(today.minusDays(1))) return "Ontem";
        return date.format(DateTimeFormatter.ofPattern("d 'de' MMMM 'de' yyyy", new Locale("pt", "BR")));
    }
}
