import Foundation
import SymairaKeychain

struct KeychainHelper {
    static let shared = KeychainHelper()

    // Legacy service name kept so tokens stored before the symaira-appkit
    // migration remain readable (dev.symaira.* is the namespace for new apps).
    private let keychain = SymairaKeychain(service: "com.symaira.memory")
    private let account = "api_token"

    @discardableResult
    func save(_ token: String) -> Bool {
        (try? keychain.save(token, key: account)) ?? false
    }

    func read() -> String? {
        (try? keychain.read(key: account)) ?? nil
    }

    func delete() {
        keychain.delete(key: account)
    }
}
