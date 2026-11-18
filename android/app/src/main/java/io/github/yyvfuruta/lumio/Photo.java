package io.github.yyvfuruta.lumio;

import org.json.JSONObject;

final class Photo {
    final String id, name, type, addedAt;
    final long size;
    final int width, height;
    final boolean favorite;
    final double duration;
    Photo(JSONObject json) throws Exception {
        id = json.getString("id");
        if (!id.matches("[a-f0-9]{64}")) throw new Exception("O servidor retornou um identificador inválido.");
        name = json.getString("name"); type = json.getString("type"); addedAt = json.getString("addedAt");
        java.time.OffsetDateTime.parse(addedAt);
        size = json.getLong("size"); width = json.getInt("width"); height = json.getInt("height");
        favorite = json.getBoolean("favorite"); duration = json.optDouble("duration", 0);
    }
    boolean video() { return type.startsWith("video/"); }
    String durationLabel() { int seconds = (int) duration; return String.format(java.util.Locale.ROOT, "%d:%02d", seconds / 60, seconds % 60); }
}
