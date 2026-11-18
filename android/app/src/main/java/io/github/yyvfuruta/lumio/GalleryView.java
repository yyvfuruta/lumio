package io.github.yyvfuruta.lumio;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.res.ColorStateList;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.TextWatcher;
import android.view.ContextThemeWrapper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.inputmethod.EditorInfo;
import android.widget.AbsListView;
import android.widget.BaseAdapter;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageButton;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ListView;
import android.widget.PopupMenu;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import org.json.JSONObject;
import java.time.YearMonth;
import java.time.format.TextStyle;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;

/** Native Android gallery, mirroring the web library's layout and operations. */
final class GalleryView extends LinearLayout implements GalleryModel.Observer {
    interface Host {
        void upload(); void download(String target, String name, String mime); void logout(); void expired(String server, String token); void theme(GalleryTheme theme);
    }
    private final Activity activity;
    private final UploadModel uploads;
    private final Host host;
    final GalleryModel model;
    private final PhotoLoader loader;
    private GalleryTheme theme;
    private final Handler debounce = new Handler(Looper.getMainLooper());
    private final LinearLayout header, photoPanel, timeline, selectionBar, statusTable;
    private final FrameLayout photoArea;
    private final ListView list;
    private final RowAdapter adapter = new RowAdapter();
    private final EditText search;
    private final ImageButton menu, upload;
    private final TextView message, count, emptyTitle, emptyText;
    private final Button emptyUpload;
    private final LinearLayout empty;
    private final ProgressBar loading;
    private final ScrollView statusPanel;
    private final List<Button> selectionButtons = new ArrayList<>();
    private final List<Row> rows = new ArrayList<>();
    private PhotoViewer viewer;
    private boolean showingStatus, closed;
    private int columns = 3, cellSize = 100;
    private String timelineKey = "";
    private final Runnable queryChanged;

    GalleryView(Activity activity, UploadModel uploads, Host host) {
        super(activity); this.activity = activity; this.uploads = uploads; this.host = host;
        theme = new GalleryTheme(activity, uploads.prefs.getString("native-theme", "system"));
        model = new GalleryModel(uploads.endpoint(), uploads.sessionToken(), this);
        loader = new PhotoLoader(activity, model.server, model.token);
        setOrientation(VERTICAL);
        header = new LinearLayout(activity); header.setGravity(Gravity.CENTER_VERTICAL); header.setPadding(dp(12), dp(10), dp(12), dp(10));
        menu = icon("menu", "Menu da biblioteca"); header.addView(menu, new LayoutParams(dp(36), dp(40))); menu.setOnClickListener(v -> menu());
        search = new EditText(activity); search.setSingleLine(true); search.setTextSize(14); search.setHint("Buscar fotos e vídeos"); search.setContentDescription("Buscar por nome do arquivo"); search.setImeOptions(EditorInfo.IME_ACTION_SEARCH);
        LayoutParams searchParams = new LayoutParams(0, dp(42), 1); searchParams.setMargins(dp(8), 0, dp(8), 0); header.addView(search, searchParams); search.setPadding(dp(12), 0, dp(12), 0);
        queryChanged = () -> model.filter(search.getText().toString().trim(), model.favorite, model.period);
        search.addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence s, int start, int n, int after) { }
            @Override public void onTextChanged(CharSequence s, int start, int before, int n) { debounce.removeCallbacks(queryChanged); debounce.postDelayed(queryChanged, 250); }
            @Override public void afterTextChanged(Editable text) { }
        });
        upload = icon("upload", "Enviar fotos e vídeos"); header.addView(upload, new LayoutParams(dp(36), dp(40))); upload.setOnClickListener(v -> host.upload()); addView(header);
        message = label("", 13, theme.muted); message.setPadding(dp(16), dp(8), dp(16), dp(8)); message.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE); message.setOnClickListener(v -> refresh()); addView(message);
        photoPanel = new LinearLayout(activity); photoPanel.setPadding(dp(12), dp(12), dp(8), dp(8)); addView(photoPanel, new LayoutParams(-1, 0, 1));
        photoArea = new FrameLayout(activity); photoPanel.addView(photoArea, new LayoutParams(0, -1, 1));
        list = new ListView(activity); list.setDivider(null); list.setClipToPadding(false); list.setPadding(0, 0, 0, dp(16)); photoArea.addView(list, new FrameLayout.LayoutParams(-1, -1));
        loading = new ProgressBar(activity); LinearLayout footer = new LinearLayout(activity); footer.setGravity(Gravity.CENTER); footer.addView(loading, new LayoutParams(dp(28), dp(28))); list.addFooterView(footer, null, false); list.setAdapter(adapter);
        list.setOnScrollListener(new AbsListView.OnScrollListener() {
            @Override public void onScrollStateChanged(AbsListView view, int state) { }
            @Override public void onScroll(AbsListView view, int first, int visible, int total) { if (!closed && total > 1 && visible > 0 && first + visible >= total - 2 && !model.mutating) model.load(false); }
        });
        empty = new LinearLayout(activity); empty.setGravity(Gravity.CENTER); empty.setOrientation(VERTICAL); empty.setPadding(dp(16), dp(40), dp(16), dp(40));
        emptyTitle = label("", 20, theme.text); empty.addView(emptyTitle); emptyText = label("", 14, theme.muted); emptyText.setGravity(Gravity.CENTER); empty.addView(emptyText);
        emptyUpload = button("Enviar fotos e vídeos", v -> host.upload()); empty.addView(emptyUpload); photoArea.addView(empty, new FrameLayout.LayoutParams(-1, -1));
        ScrollView dates = new ScrollView(activity); dates.setFillViewport(false); timeline = new LinearLayout(activity); timeline.setOrientation(VERTICAL); dates.addView(timeline); photoPanel.addView(dates, new LayoutParams(dp(44), -1));
        selectionBar = new LinearLayout(activity); selectionBar.setPadding(dp(8), dp(6), dp(8), dp(6)); selectionBar.setOrientation(VERTICAL); count = label("", 13, theme.text); selectionBar.addView(count);
        LinearLayout first = new LinearLayout(activity), second = new LinearLayout(activity); selectionBar.addView(first); selectionBar.addView(second);
        selectionButton(first, "Tudo", v -> model.selectAll(null)); selectionButton(first, "Limpar", v -> model.clearSelection()); selectionButton(first, "Cancelar", v -> model.cancelSelection());
        selectionButton(second, "Baixar ZIP", v -> model.archive(host::download)); selectionButton(second, "Excluir", v -> confirmDelete(new ArrayList<>(model.selection.keySet())));
        addView(selectionBar);
        statusPanel = new ScrollView(activity); statusTable = new LinearLayout(activity); statusTable.setOrientation(VERTICAL); statusTable.setPadding(dp(20), dp(20), dp(20), dp(20)); statusPanel.addView(statusTable); addView(statusPanel, new LayoutParams(-1, 0, 1)); statusPanel.setVisibility(GONE);
        applyTheme(); model.load(true);
    }
    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }
    private TextView label(String value, int size, int color) { TextView v = new TextView(activity); v.setText(value); v.setTextSize(size); v.setTextColor(color); return v; }
    private GradientDrawable background(int color, int radius) { GradientDrawable bg = new GradientDrawable(); bg.setColor(color); bg.setCornerRadius(dp(radius)); return bg; }
    private ImageButton icon(String kind, String description) { ImageButton b = new ImageButton(activity); b.setImageDrawable(new GalleryIcon(kind, theme.text)); b.setContentDescription(description); b.setBackgroundColor(Color.TRANSPARENT); b.setPadding(dp(8), dp(8), dp(8), dp(8)); return b; }
    private Button button(String title, OnClickListener action) { Button b = new Button(activity); b.setText(title); b.setAllCaps(false); b.setTextSize(13); b.setTextColor(theme.text); b.setBackgroundTintList(ColorStateList.valueOf(theme.surface)); b.setOnClickListener(action); return b; }
    private void selectionButton(LinearLayout parent, String text, OnClickListener action) { Button b = button(text, action); parent.addView(b, new LayoutParams(0, dp(44), 1)); selectionButtons.add(b); }
    private void menu() {
        PopupMenu popup = new PopupMenu(new ContextThemeWrapper(activity, theme.dark ? android.R.style.Theme_Material : android.R.style.Theme_Material_Light), menu);
        popup.getMenu().add("Fotos").setOnMenuItemClickListener(item -> { showPhotos(false); return true; });
        popup.getMenu().add("Favoritos").setOnMenuItemClickListener(item -> { showPhotos(true); return true; });
        popup.getMenu().add("Armazenamento e status").setOnMenuItemClickListener(item -> { showingStatus = true; changed(); model.loadStatus(); return true; });
        popup.getMenu().add("Aparência").setOnMenuItemClickListener(item -> {
            String[] values = {"system", "light", "dark"}; String current = uploads.prefs.getString("native-theme", "system"); int selected = java.util.Arrays.asList(values).indexOf(current);
            new AlertDialog.Builder(activity, theme.dialogTheme()).setTitle("Aparência").setSingleChoiceItems(new String[]{"Sistema", "Claro", "Escuro"}, selected, (dialog, index) -> {
                uploads.prefs.edit().putString("native-theme", values[index]).apply(); theme = new GalleryTheme(activity, values[index]); timelineKey = ""; applyTheme(); changed(); dialog.dismiss();
            }).show(); return true;
        });
        popup.getMenu().add("Atualizar").setOnMenuItemClickListener(item -> { refresh(); return true; });
        popup.getMenu().add("Sair").setEnabled(!model.transferring).setOnMenuItemClickListener(item -> { host.logout(); return true; });
        popup.show();
    }
    private void showPhotos(boolean favorites) { showingStatus = false; model.filter(search.getText().toString().trim(), favorites, model.period); list.setSelection(0); }
    void shown() { host.theme(theme); }
    void refresh() { if (showingStatus) model.loadStatus(); else if (!model.mutating) model.load(true); }
    void transfers(boolean busy) { if (model.transferring != busy) { model.transferring = busy; changed(); } upload.setEnabled(!busy && !model.mutating); emptyUpload.setEnabled(!busy); }
    private void applyTheme() {
        setBackgroundColor(theme.background); header.setBackgroundColor(theme.background); list.setBackgroundColor(theme.background);
        menu.setImageDrawable(new GalleryIcon("menu", theme.text)); upload.setImageDrawable(new GalleryIcon("upload", theme.text));
        search.setTextColor(theme.text); search.setHintTextColor(theme.muted); search.setBackground(background(theme.surface, 8));
        message.setTextColor(theme.muted); emptyTitle.setTextColor(theme.text); emptyText.setTextColor(theme.muted); emptyUpload.setTextColor(theme.text); emptyUpload.setBackgroundTintList(ColorStateList.valueOf(theme.surface));
        count.setTextColor(theme.text); selectionBar.setBackground(background(theme.surface, 8));
        for (Button b : selectionButtons) { b.setTextColor(theme.text); b.setBackgroundTintList(ColorStateList.valueOf(theme.surface)); }
        host.theme(theme);
    }
    @Override public void changed() {
        if (closed) return;
        message.setText(model.message); message.setVisibility(model.message.isEmpty() ? GONE : VISIBLE);
        upload.setEnabled(!model.transferring && !model.mutating);
        search.setEnabled(!model.mutating); menu.setEnabled(!model.mutating);
        search.setVisibility(showingStatus ? GONE : VISIBLE); photoPanel.setVisibility(showingStatus ? GONE : VISIBLE); statusPanel.setVisibility(showingStatus ? VISIBLE : GONE);
        selectionBar.setVisibility(model.selecting && !showingStatus ? VISIBLE : GONE); count.setText(model.selection.size() + " selecionado(s)");
        for (Button b : selectionButtons) b.setEnabled(!model.mutating);
        selectionButtons.get(3).setEnabled(!model.mutating && !model.transferring && !model.selection.isEmpty()); selectionButtons.get(4).setEnabled(!model.mutating && !model.transferring && !model.selection.isEmpty());
        loading.setVisibility(model.loading ? VISIBLE : GONE);
        empty.setVisibility(model.photos.isEmpty() && !model.loading ? VISIBLE : GONE);
        emptyTitle.setText(!model.message.isEmpty() ? "Não foi possível carregar" : !model.query.isEmpty() ? "Nenhum resultado" : model.favorite ? "Nenhum favorito ainda" : "Nenhuma foto ainda");
        emptyText.setText(!model.query.isEmpty() ? "Tente outro nome de arquivo." : model.favorite ? "Toque no coração de uma foto ou vídeo." : "Envie fotos e vídeos para começar."); emptyUpload.setVisibility(model.favorite || !model.query.isEmpty() ? GONE : VISIBLE);
        rebuildRows(); adapter.notifyDataSetChanged(); renderTimeline();
        if (showingStatus) renderStatus();
        if (viewer != null && viewer.isShowing()) viewer.refresh();
    }
    @Override public void expired() { host.expired(model.server, model.token); }
    private void rebuildRows() {
        rows.clear(); LinkedHashMap<String, List<Photo>> groups = new LinkedHashMap<>();
        for (Photo p : model.photos) { String day = LibraryDates.day(p.addedAt, model.zone); groups.computeIfAbsent(day, key -> new ArrayList<>()).add(p); }
        for (java.util.Map.Entry<String, List<Photo>> group : groups.entrySet()) {
            rows.add(new Row(group.getKey(), null)); List<Photo> photos = group.getValue();
            for (int i = 0; i < photos.size(); i += columns) rows.add(new Row(group.getKey(), new ArrayList<>(photos.subList(i, Math.min(photos.size(), i + columns)))));
        }
    }
    @Override protected void onSizeChanged(int w, int h, int oldW, int oldH) {
        super.onSizeChanged(w, h, oldW, oldH); columns = w / getResources().getDisplayMetrics().density >= 700 ? 5 : 3;
        cellSize = Math.max(dp(48), (w - dp(64) - dp(3) * (columns - 1)) / columns); rebuildRows(); adapter.notifyDataSetChanged();
    }
    private void renderTimeline() {
        String key = model.period + "|" + theme.dark + "|" + model.months.toString(); if (key.equals(timelineKey)) return; timelineKey = key; timeline.removeAllViews();
        dateButton("Tudo", "", false); String previousYear = "";
        for (String value : model.months) {
            YearMonth date = YearMonth.parse(value); String year = Integer.toString(date.getYear());
            if (!year.equals(previousYear)) { dateButton(year, year, true); previousYear = year; }
            dateButton(date.getMonth().getDisplayName(TextStyle.SHORT, new Locale("pt", "BR")).replace(".", ""), value, false);
        }
    }
    private void dateButton(String label, String period, boolean year) {
        TextView button = label(label, 11, period.equals(model.period) || year ? theme.text : theme.muted); button.setGravity(Gravity.CENTER); button.setPadding(0, dp(7), 0, dp(7));
        if (year) button.setTypeface(null, Typeface.BOLD); if (period.equals(model.period)) button.setBackgroundColor(theme.surface);
        button.setContentDescription(period.isEmpty() ? "Todas as datas" : "Uploads de " + period); button.setOnClickListener(v -> { if (!model.mutating) { model.filter(model.query, model.favorite, period); list.setSelection(0); } }); timeline.addView(button, new LayoutParams(-1, -2));
    }
    private void confirmDelete(List<String> ids) {
        if (ids.isEmpty() || model.mutating) return;
        new AlertDialog.Builder(activity, theme.dialogTheme()).setTitle("Excluir arquivos?").setMessage("Excluir permanentemente " + ids.size() + " arquivo(s) do servidor?")
                .setNegativeButton("Cancelar", null).setPositiveButton("Excluir", (dialog, which) -> model.delete(ids)).show();
    }
    private void open(Photo photo) { if (viewer != null) viewer.dismiss(); viewer = new PhotoViewer(activity, model, loader, theme, host::download, photo.id); viewer.show(); }
    private void renderStatus() {
        statusTable.removeAllViews(); TextView title = label("Armazenamento e status", 20, theme.text); statusTable.addView(title); statusTable.addView(button("Atualizar", v -> model.loadStatus()));
        if (model.statusLoading) { statusTable.addView(new ProgressBar(activity)); return; }
        JSONObject status = model.status; if (status == null) return;
        statusRow("Fotos e vídeos", Long.toString(status.optLong("photos"))); statusRow("Favoritos", Long.toString(status.optLong("favorites")));
        for (String[] field : new String[][]{{"Originais", "originalBytes"}, {"Miniaturas", "thumbnailBytes"}, {"Pré-visualizações", "previewBytes"}}) statusRow(field[0], android.text.format.Formatter.formatFileSize(activity, status.optLong(field[1])));
        statusRow("Exclusões pendentes", Long.toString(status.optLong("pendingDeletions"))); statusRow("Banco de dados", status.optString("database").equals("available") ? "Disponível" : status.optString("database")); statusRow("Armazenamento", status.optString("storage")); statusRow("Acesso ao armazenamento", status.optBoolean("storageAccessible") ? "Disponível" : "Indisponível");
        JSONObject disk = status.optJSONObject("disk");
        if (disk != null) for (String[] field : new String[][]{{"Capacidade", "total"}, {"Usado", "used"}, {"Disponível", "available"}}) statusRow(field[0], android.text.format.Formatter.formatFileSize(activity, disk.optLong(field[1])));
        else statusTable.addView(label("O armazenamento remoto não informa a capacidade do volume.", 12, theme.muted));
    }
    private void statusRow(String title, String value) { LinearLayout row = new LinearLayout(activity); row.setPadding(dp(4), dp(12), dp(4), dp(12)); row.addView(label(title, 13, theme.muted), new LayoutParams(0, -2, 1)); row.addView(label(value, 13, theme.text), new LayoutParams(0, -2, 1)); statusTable.addView(row); View border = new View(activity); border.setBackgroundColor(theme.border); statusTable.addView(border, new LayoutParams(-1, dp(1))); }
    boolean back() { if (showingStatus) { showingStatus = false; changed(); return true; } if (model.selecting && !model.mutating) { model.cancelSelection(); return true; } return false; }
    void pause() { if (viewer != null) viewer.pause(); }
    void close() { closed = true; debounce.removeCallbacksAndMessages(null); if (viewer != null) viewer.dismiss(); model.close(); loader.close(); }
    private static final class Row { final String day; final List<Photo> photos; Row(String day, List<Photo> photos) { this.day = day; this.photos = photos; } }
    private final class RowAdapter extends BaseAdapter {
        @Override public int getCount() { return rows.size(); }
        @Override public Object getItem(int position) { return rows.get(position); }
        @Override public long getItemId(int position) { return position; }
        @Override public int getViewTypeCount() { return 2; }
        @Override public int getItemViewType(int position) { return rows.get(position).photos == null ? 0 : 1; }
        @Override public View getView(int position, View recycled, ViewGroup parent) {
            Row row = rows.get(position);
            LinearLayout line = recycled instanceof LinearLayout ? (LinearLayout) recycled : new LinearLayout(activity); line.removeAllViews(); line.setGravity(Gravity.CENTER_VERTICAL); line.setBackgroundColor(theme.background);
            if (row.photos == null) {
                line.setPadding(dp(2), dp(8), 0, dp(8)); CheckBox day = new CheckBox(activity); day.setButtonTintList(ColorStateList.valueOf(theme.text)); day.setChecked(model.daySelected(row.day)); day.setEnabled(!model.mutating); day.setContentDescription("Selecionar uploads de " + row.day); day.setOnClickListener(v -> model.selectAll(row.day)); line.addView(day, new LayoutParams(dp(30), dp(30))); TextView date = label(LibraryDates.label(row.day, model.zone), 14, theme.text); line.addView(date); return line;
            }
            line.setPadding(0, 0, 0, dp(3));
            for (int i = 0; i < columns; i++) {
                if (i >= row.photos.size()) { line.addView(new View(activity), new LayoutParams(0, cellSize, 1)); continue; }
                Photo photo = row.photos.get(i); FrameLayout tile = new FrameLayout(activity); tile.setBackgroundColor(theme.surface); tile.setContentDescription(photo.name); LayoutParams cell = new LayoutParams(0, cellSize, 1); if (i > 0) cell.leftMargin = dp(3); line.addView(tile, cell);
                ImageView thumb = new ImageView(activity); thumb.setScaleType(ImageView.ScaleType.CENTER_CROP); thumb.setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO); tile.addView(thumb, new FrameLayout.LayoutParams(-1, -1)); loader.bind(photo, false, thumb, null);
                tile.setOnClickListener(v -> { if (model.selecting) model.toggle(photo, !model.selection.containsKey(photo.id)); else open(photo); }); tile.setOnLongClickListener(v -> { model.toggle(photo, true); tile.performHapticFeedback(android.view.HapticFeedbackConstants.LONG_PRESS); return true; });
                CheckBox selected = new CheckBox(activity); selected.setButtonTintList(ColorStateList.valueOf(Color.WHITE)); selected.setChecked(model.selection.containsKey(photo.id)); selected.setEnabled(!model.mutating); selected.setContentDescription("Selecionar " + photo.name); selected.setOnClickListener(v -> model.toggle(photo, selected.isChecked())); FrameLayout.LayoutParams selectParams = new FrameLayout.LayoutParams(dp(30), dp(30), Gravity.TOP | Gravity.LEFT); selectParams.setMargins(dp(3), dp(3), 0, 0); tile.addView(selected, selectParams);
                if (model.selection.containsKey(photo.id)) { GradientDrawable outline = new GradientDrawable(); outline.setColor(Color.TRANSPARENT); outline.setStroke(dp(3), Color.WHITE); tile.setForeground(outline); }
                ImageButton heart = icon(photo.favorite ? "saved" : "heart", (photo.favorite ? "Remover favorito " : "Favoritar ") + photo.name); heart.setImageDrawable(new GalleryIcon(photo.favorite ? "saved" : "heart", Color.WHITE)); heart.setBackground(background(0x44000000, 3)); heart.setPadding(dp(5), dp(5), dp(5), dp(5)); heart.setEnabled(!model.favoriteBusy.contains(photo.id) && !model.mutating); heart.setOnClickListener(v -> model.favorite(photo)); FrameLayout.LayoutParams heartParams = new FrameLayout.LayoutParams(dp(30), dp(30), Gravity.BOTTOM | Gravity.LEFT); heartParams.setMargins(dp(4), 0, 0, dp(4)); tile.addView(heart, heartParams);
                if (photo.video()) { TextView badge = label("▶ " + photo.durationLabel(), 11, Color.WHITE); badge.setPadding(dp(4), dp(2), dp(4), dp(2)); badge.setBackground(background(0x99000000, 3)); FrameLayout.LayoutParams badgeParams = new FrameLayout.LayoutParams(-2, -2, Gravity.BOTTOM | Gravity.RIGHT); badgeParams.setMargins(0, 0, dp(4), dp(5)); tile.addView(badge, badgeParams); }
            }
            return line;
        }
    }
}
