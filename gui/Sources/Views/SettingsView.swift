import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var client: APIClient
    @State private var serverURLInput = ""
    @State private var tokenInput = ""
    @State private var showToken = false
    @State private var isSaveHovered = false
    
    var body: some View {
        ZStack {
            Color.bgDark.ignoresSafeArea()
            
            // Background Ambient Glow
            RadialGradient(
                colors: [Color.goldPrimary.opacity(0.04), Color.clear],
                center: .top,
                startRadius: 0,
                endRadius: 400
            )
            .ignoresSafeArea()
            
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Server Configuration")
                            .font(.system(.title, design: .rounded))
                            .fontWeight(.bold)
                            .foregroundColor(.textPrimary)
                        Text("Configure the local Symaira Memory daemon details below.")
                            .font(.subheadline)
                            .foregroundColor(.textSecondary)
                    }
                    .padding(.bottom, 8)
                    
                    // Form Section - Connection Settings
                    VStack(alignment: .leading, spacing: 16) {
                        Text("CONNECTION SETTINGS")
                            .font(.system(size: 11, weight: .bold, design: .rounded))
                            .foregroundColor(.goldPrimary)
                        
                        VStack(alignment: .leading, spacing: 6) {
                            Text("Server URL")
                                .font(.caption)
                                .foregroundColor(.textSecondary)
                            TextField("http://127.0.0.1:8787", text: $serverURLInput)
                                .textFieldStyle(.plain)
                                .padding(8)
                                .background(Color.bgDarker)
                                .foregroundColor(.textPrimary)
                                .cornerRadius(6)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 6)
                                        .stroke(Color.borderGlass, lineWidth: 1)
                                )
                                .autocorrectionDisabled()
                        }
                        
                        VStack(alignment: .leading, spacing: 6) {
                            Text("API Token (JWT)")
                                .font(.caption)
                                .foregroundColor(.textSecondary)
                            HStack {
                                if showToken {
                                    TextField("Token string", text: $tokenInput)
                                        .textFieldStyle(.plain)
                                        .foregroundColor(.textPrimary)
                                } else {
                                    SecureField("Token string", text: $tokenInput)
                                        .textFieldStyle(.plain)
                                        .foregroundColor(.textPrimary)
                                }
                                
                                Button {
                                    showToken.toggle()
                                } label: {
                                    Image(systemName: showToken ? "eye.slash" : "eye")
                                        .foregroundColor(.goldPrimary)
                                }
                                .buttonStyle(.plain)
                            }
                            .padding(8)
                            .background(Color.bgDarker)
                            .cornerRadius(6)
                            .overlay(
                                RoundedRectangle(cornerRadius: 6)
                                    .stroke(Color.borderGlass, lineWidth: 1)
                            )
                        }
                        
                        Button {
                            client.serverURL = serverURLInput.trimmingCharacters(in: .whitespacesAndNewlines)
                            client.token = tokenInput.trimmingCharacters(in: .whitespacesAndNewlines)
                        } label: {
                            Text("Save and Verify Connection")
                                .fontWeight(.semibold)
                                .foregroundColor(.bgDarker)
                                .padding(.horizontal, 20)
                                .padding(.vertical, 10)
                                .background(Color.goldPrimary)
                                .cornerRadius(8)
                                .shadow(color: Color.goldPrimary.opacity(0.3), radius: 4, x: 0, y: 2)
                        }
                        .buttonStyle(.plain)
                        .scaleEffect(isSaveHovered ? 1.02 : 1.0)
                        .onHover { hovering in
                            withAnimation(.easeInOut(duration: 0.15)) {
                                isSaveHovered = hovering
                            }
                        }
                        .padding(.top, 8)
                    }
                    .padding()
                    .background(Color.cardBackground)
                    .cornerRadius(12)
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .stroke(Color.borderGlass, lineWidth: 1)
                    )
                    
                    // Connection Status panel
                    VStack(alignment: .leading, spacing: 12) {
                        Text("DAEMON CONNECTION STATUS")
                            .font(.system(size: 11, weight: .bold, design: .rounded))
                            .foregroundColor(.goldPrimary)
                        
                        HStack(spacing: 8) {
                            Text("Status:")
                                .foregroundColor(.textSecondary)
                            
                            statusBadge
                        }
                        
                        if case let .failed(errorMsg) = client.connectionStatus {
                            Text(errorMsg)
                                .font(.caption)
                                .foregroundColor(.red)
                                .padding(.top, 4)
                        }
                    }
                    .padding()
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.cardBackground)
                    .cornerRadius(12)
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .stroke(Color.borderGlass, lineWidth: 1)
                    )
                    
                    // Developer Guide
                    VStack(alignment: .leading, spacing: 12) {
                        Text("DEVELOPER INSTRUCTIONS")
                            .font(.system(size: 11, weight: .bold, design: .rounded))
                            .foregroundColor(.goldPrimary)
                        
                        Text("Symaira Memory requires a Bearer JWT Token to authenticate. If you do not have one, you can generate a token locally via the CLI by running this command in your repository:")
                            .font(.subheadline)
                            .foregroundColor(.textSecondary)
                            .lineSpacing(4)
                        
                        Text("./symmemory token generate --subject \"desktop-gui\"")
                            .font(.system(.body, design: .monospaced))
                            .foregroundColor(.goldSecondary)
                            .padding(12)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.bgDarker)
                            .cornerRadius(6)
                            .overlay(
                                RoundedRectangle(cornerRadius: 6)
                                    .stroke(Color.borderGlass, lineWidth: 1)
                            )
                            .textSelection(.enabled)
                    }
                    .padding()
                    .background(Color.cardBackground)
                    .cornerRadius(12)
                    .overlay(
                        RoundedRectangle(cornerRadius: 12)
                            .stroke(Color.borderGlass, lineWidth: 1)
                    )
                }
                .padding(24)
            }
        }
        .onAppear {
            serverURLInput = client.serverURL
            tokenInput = client.token
        }
    }
    
    @ViewBuilder
    private var statusBadge: some View {
        switch client.connectionStatus {
        case .disconnected:
            HStack(spacing: 4) {
                Circle().fill(Color.gray).frame(width: 8, height: 8)
                Text("Disconnected").foregroundColor(.textSecondary)
            }
        case .connecting:
            HStack(spacing: 6) {
                ProgressView().controlSize(.mini)
                Text("Connecting...").foregroundColor(.textSecondary)
            }
        case .connected(let version):
            HStack(spacing: 4) {
                Circle().fill(Color.goldPrimary).frame(width: 8, height: 8).shadow(color: .goldPrimary, radius: 4)
                Text("Connected (v\(version))").foregroundColor(.goldPrimary).fontWeight(.semibold)
            }
        case .failed:
            HStack(spacing: 4) {
                Circle().fill(Color.red).frame(width: 8, height: 8).shadow(color: .red, radius: 4)
                Text("Failed").foregroundColor(.red).fontWeight(.semibold)
            }
        }
    }
}
