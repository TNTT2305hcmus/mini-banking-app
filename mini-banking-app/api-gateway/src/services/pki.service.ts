import { CAServiceClient } from "../proto/ca";
import { sslCredentials } from "../config/grpc";

const caServiceClient = new CAServiceClient("localhost:50052", sslCredentials);
