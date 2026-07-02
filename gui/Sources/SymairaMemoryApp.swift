import SwiftUI

@main
struct SymairaMemoryApp: App {
    @StateObject private var client = APIClient()
    
    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(client)
        }
        .windowStyle(.hiddenTitleBar)
    }
}
