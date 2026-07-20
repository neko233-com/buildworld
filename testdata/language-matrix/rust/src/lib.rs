pub fn message(name: &str) -> String {
    format!("hello, {name}")
}

#[cfg(test)]
mod tests {
    use super::message;

    #[test]
    fn formats_message() {
        assert_eq!(message("matrix"), "hello, matrix");
    }
}
