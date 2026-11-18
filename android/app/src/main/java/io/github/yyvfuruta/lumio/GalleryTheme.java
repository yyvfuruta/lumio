package io.github.yyvfuruta.lumio;

import android.content.Context;
import android.content.res.Configuration;
import android.graphics.Color;

final class GalleryTheme {
    final boolean dark;
    final int background, surface, text, muted, border;
    GalleryTheme(Context context, String preference) {
        dark = preference.equals("dark") || (preference.equals("system") && (context.getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES);
        background = Color.parseColor(dark ? "#171717" : "#ffffff"); surface = Color.parseColor(dark ? "#232323" : "#f5f5f5");
        text = Color.parseColor(dark ? "#f3f3f3" : "#171717"); muted = Color.parseColor(dark ? "#a4a4a4" : "#777777"); border = Color.parseColor(dark ? "#353535" : "#e6e6e6");
    }
    int dialogTheme() { return dark ? android.R.style.Theme_Material_Dialog_Alert : android.R.style.Theme_Material_Light_Dialog_Alert; }
}
