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
                        .symairaText(.body, respectsForeground: false)
                        .padding(.vertical, SymairaSpacing.xSmall)
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
                    HStack(spacing: SymairaSpacing.small) {
                        connectionStatusDot
                        Text(connectionStatusText)
                            .symairaText(.caption)
                            .fontWeight(.medium)
                            .foregroundColor(.textSecondary)
                            .lineLimit(1)
                        Spacer()
                    }
                    .padding(SymairaSpacing.medium)
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
            SymairaStatusDot(tone: .positive, accessibilityLabel: connectionStatusText)
        case .connecting:
            ProgressView().controlSize(.mini).scaleEffect(0.6)
        case .disconnected:
            SymairaStatusDot(tone: .neutral, accessibilityLabel: connectionStatusText)
        case .failed:
            SymairaStatusDot(tone: .critical, accessibilityLabel: connectionStatusText)
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
