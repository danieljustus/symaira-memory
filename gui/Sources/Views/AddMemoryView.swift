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
                    .symairaText(.bodyEmphasized)
                    .foregroundColor(.textPrimary)
                Spacer()
                Button(action: { dismiss() }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundColor(.textSecondary)
                        .symairaText(.heading)
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
                    .foregroundColor(SymairaTheme.critical)
                    .symairaText(.callout)
                    .padding()
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(SymairaTheme.critical.opacity(0.1))
                    .cornerRadius(8)
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(SymairaTheme.critical.opacity(0.2), lineWidth: 1))
                }
                
                Text("Scope")
                    .symairaText(.callout)
                    .fontWeight(.medium)
                    .foregroundColor(.textSecondary)
                
                Picker("", selection: $selectedScope) {
                    ForEach(scopes, id: \.self) { scope in
                        Text(scope.capitalized).tag(scope)
                    }
                }
                .pickerStyle(.segmented)
                
                Text("Memory Content")
                    .symairaText(.callout)
                    .fontWeight(.medium)
                    .foregroundColor(.textSecondary)
                
                TextEditor(text: $content)
                    .symairaText(.body)
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
                    .symairaText(.caption)
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
