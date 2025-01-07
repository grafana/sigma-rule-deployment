from settings import load_config

if __name__ == "__main__":
    config = load_config()
    print(config.as_dict())
