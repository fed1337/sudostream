import {defineConfig} from "allure";
import {env} from "node:process";

export default defineConfig({
    name: "sudoStream Tests",
    output: "./allure-report",
    historyLimit: 30,
    plugins: {
        awesome: {
            options: {
                reportName: "sudoStream Allure Awesome Report",
                publish: env.ALLURE_SERVICE_TOKEN,
            },
        },
        dashboard: {
            options: {
                reportName: "sudoStream Allure Dashboard Report",
                publish: env.ALLURE_SERVICE_TOKEN,
            },
        },
    },
    allureService: {
        accessToken: env.ALLURE_SERVICE_TOKEN,
    },
});
