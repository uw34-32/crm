// The app is the Expo web export; this only opens it in a window. It talks to the API
// from the page itself, so there are no commands of its own.

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running the CRM desktop app");
}
