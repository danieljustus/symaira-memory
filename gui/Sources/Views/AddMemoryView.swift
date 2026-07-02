import SwiftUI

struct AddMemoryView: View {
    @EnvironmentObject var client: APIClient
    @Environment(\.dismiss) var dismiss
    
    @State private var content = ""
    @State private var selectedScope = "global"
    @State private var isSaving = false
    @State private var errorMessage: String? = nil
    
    let scopes = ["global", "project", "agent", "user", "session"]
    
    @State private var isSaveHovered = false
    
    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Text("Add New Memory")
                    .font(.system(.headline, design: .rounded))
                    .foregroundColor(.textPrimary)
                Spacer()
                Button(action: { dismiss() }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundColor(.textSecondary)
                        .font(.title2)
                }
                .buttonStyle(.plain)
            }
            .padding()
            .background(Color.bgDarker)
            
            Divider()
                .background(Color.borderGlass)
            
            // Content
            VStack(alignment: .leading, spacing: 16) {
                if let error = errorMessage {
                    HStack {
                        Image(systemName: "exclamationmark.octagon.fill")
                        Text(error)
                    }
                    .foregroundColor(.red)
                    .font(.subheadline)
                    .padding()
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.red.opacity(0.1))
                    .cornerRadius(8)
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.red.opacity(0.2), lineWidth: 1))
                }
                
                Text("Scope")
                    .font(.subheadline)
                    .fontWeight(.medium)
                    .foregroundColor(.textSecondary)
                
                Picker("", selection: $selectedScope) {
                    ForEach(scopes, id: \.self) { scope in
                        Text(scope.capitalized).tag(scope)
                    }
                }
                .pickerStyle(.segmented)
                
                Text("Memory Content")
                    .font(.subheadline)
                    .fontWeight(.medium)
                    .foregroundColor(.textSecondary)
                
                TextEditor(text: $content)
                    .font(.body)
                    .foregroundColor(.textPrimary)
                    .padding(6)
                    .scrollContentBackground(.hidden)
                    .background(Color.bgDarker)
                    .cornerRadius(6)
                    .overlay(
                        RoundedRectangle(cornerRadius: 6)
                            .stroke(Color.borderGlass, lineWidth: 1)
                    )
                    .frame(minHeight: 120)
                
                Text("Example: 'User prefers using Go and Python for API services'.")
                    .font(.caption)
                    .foregroundColor(.textMuted)
            }
            .padding()
            .background(Color.bgDark)
            
            Spacer()
            
            Divider()
                .background(Color.borderGlass)
            
            // Actions
            HStack {
                Spacer()
                
                Button("Cancel") {
                    dismiss()
                }
                .buttonStyle(.plain)
                .foregroundColor(.textSecondary)
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .background(Color.clear)
                .cornerRadius(6)
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(Color.borderGlass, lineWidth: 1))
                
                Button(action: saveMemory) {
                    HStack {
                        if isSaving {
                            ProgressView().controlSize(.mini)
                        } else {
                            Text("Save Memory")
                        }
                    }
                    .fontWeight(.semibold)
                    .foregroundColor(.bgDarker)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 8)
                    .background(content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isSaving ? Color.goldPrimary.opacity(0.5) : Color.goldPrimary)
                    .cornerRadius(6)
                }
                .buttonStyle(.plain)
                .disabled(content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isSaving)
                .scaleEffect(isSaveHovered ? 1.02 : 1.0)
                .onHover { hovering in
                    if !content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !isSaving {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            isSaveHovered = hovering
                        }
                    }
                }
            }
            .padding()
            .background(Color.bgDarker)
        }
        .frame(width: 450, height: 420)
    }
    
    private func saveMemory() {
        guard !content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        isSaving = true
        errorMessage = nil
        
        Task {
            let success = await client.saveMemory(
                content: content.trimmingCharacters(in: .whitespacesAndNewlines),
                scope: selectedScope
            )
            isSaving = false
            if success {
                dismiss()
            } else {
                errorMessage = client.errorMessage ?? "An error occurred while saving the memory."
            }
        }
    }
}
