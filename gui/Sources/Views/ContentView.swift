import SwiftUI

enum NavigationItem: String, CaseIterable, Identifiable {
    case dashboard = "Dashboard"
    case memories = "Memories"
    case rules = "Rules"
    case settings = "Settings"
    
    var id: String { self.rawValue }
    
    var iconName: String {
        switch self {
        case .dashboard: return "square.grid.2x2.fill"
        case .memories: return "brain"
        case .rules: return "doc.text.fill"
        case .settings: return "gearshape.fill"
        }
    }
}

struct ContentView: View {
    @EnvironmentObject var client: APIClient
    @State private var selectedItem: NavigationItem? = .dashboard
    
    var body: some View {
        NavigationSplitView {
            List(NavigationItem.allCases, selection: $selectedItem) { item in
                NavigationLink(value: item) {
                    Label(item.rawValue, systemImage: item.iconName)
                        .font(.system(.body, design: .rounded))
                        .padding(.vertical, 4)
                }
                .listRowSeparator(.hidden)
            }
            .scrollContentBackground(.hidden)
            .background(Color.bgDarker)
            .navigationTitle("Symaira Memory")
            .frame(minWidth: 200)
            
            // Connection Status Panel in sidebar footer
            .safeAreaInset(edge: .bottom) {
                VStack(spacing: 0) {
                    Divider()
                        .background(Color.borderGlass)
                    HStack(spacing: 8) {
                        connectionStatusDot
                        Text(connectionStatusText)
                            .font(.system(size: 11, weight: .medium, design: .rounded))
                            .foregroundColor(.textSecondary)
                            .lineLimit(1)
                        Spacer()
                    }
                    .padding(12)
                    .background(Color.bgDarker)
                }
            }
        } detail: {
            Group {
                if let selectedItem {
                    switch selectedItem {
                    case .dashboard:
                        DashboardView()
                            .environmentObject(client)
                    case .memories:
                        MemoriesView()
                            .environmentObject(client)
                    case .rules:
                        RulesView()
                            .environmentObject(client)
                    case .settings:
                        SettingsView()
                            .environmentObject(client)
                    }
                } else {
                    Text("Select an item from the sidebar")
                        .foregroundColor(.textSecondary)
                }
            }
            .background(Color.bgDark)
        }
        .frame(minWidth: 850, minHeight: 600)
        .foregroundColor(.textPrimary)
    }
    
    @ViewBuilder
    private var connectionStatusDot: some View {
        switch client.connectionStatus {
        case .connected:
            Circle()
                .fill(Color.goldPrimary)
                .frame(width: 8, height: 8)
                .shadow(color: .goldPrimary, radius: 4)
        case .connecting:
            ProgressView().controlSize(.mini).scaleEffect(0.6)
        case .disconnected, .failed:
            Circle()
                .fill(Color.red)
                .frame(width: 8, height: 8)
                .shadow(color: .red, radius: 4)
        }
    }
    
    private var connectionStatusText: String {
        switch client.connectionStatus {
        case .connected(let version):
            return "Active Daemon (v\(version))"
        case .connecting:
            return "Checking Connection..."
        case .disconnected:
            return "Disconnected"
        case .failed:
            return "Daemon Offline"
        }
    }
}
