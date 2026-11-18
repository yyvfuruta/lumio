package io.github.yyvfuruta.lumio;

import java.net.URI;

/** Origin and credential rules shared by the WebView adapter and downloads. */
public final class WebPolicy {
    private WebPolicy() { }
    public static boolean sameOrigin(String server, String target) {
        try {
            URI a = URI.create(server), b = URI.create(target);
            return a.getHost() != null && b.getHost() != null && b.getRawUserInfo() == null
                    && a.getScheme().equalsIgnoreCase(b.getScheme())
                    && a.getHost().equalsIgnoreCase(b.getHost()) && port(a) == port(b);
        } catch (IllegalArgumentException | NullPointerException e) { return false; }
    }
    private static int port(URI uri) {
        return uri.getPort() >= 0 ? uri.getPort() : "https".equalsIgnoreCase(uri.getScheme()) ? 443 : 80;
    }
    public static boolean downloadAllowed(String server, String target) {
        if (!sameOrigin(server, target)) return false;
        String path = URI.create(target).getRawPath();
        return path != null && (path.matches("/api/v1/photos/[a-f0-9]{64}/original")
                || path.matches("/api/v1/photo-archives/[a-f0-9]{32}"));
    }
    public static String sessionToken(String cookies) {
        if (cookies != null) for (String cookie : cookies.split(";")) {
            String[] pair = cookie.trim().split("=", 2);
            if (pair.length == 2 && pair[0].equals("lumio_session") && pair[1].matches("[a-f0-9]{64}")) return pair[1];
        }
        return "";
    }
    public static String filename(String raw) {
        String name = raw == null ? "download" : raw.replace('\\', '/');
        name = name.substring(name.lastIndexOf('/') + 1).replaceAll("[\\p{Cntrl}]", "_").trim();
        if (name.isEmpty() || name.equals(".") || name.equals("..")) name = "download";
        return name.length() > 200 ? name.substring(0, 200) : name;
    }
}
