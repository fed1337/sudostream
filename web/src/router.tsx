import { createBrowserRouter, Navigate } from "react-router";
import { AdminRoute, GuestRoute, ProtectedRoute } from "./layouts/AuthGuards";
import { DashboardLayout } from "./layouts/DashboardLayout";
import LoginPage from "./routes/login";
import LoginTwoFactorPage from "./routes/login/2fa";
import ForgotPasswordPage from "./routes/forgot-password";
import ResetPasswordPage from "./routes/reset-password";
import ConfirmEmailPage from "./routes/confirm-email";
import AcceptInvitePage from "./routes/accept-invite";
import HomePage from "./routes/home";
import { ContinuePage, FavoritesPage, UnwatchedPage, WatchedPage } from "./routes/shelf";
import LibrariesPage from "./routes/libraries";
import UserLayout from "./routes/user";
import UserIndexPage from "./routes/user/index";
import UserChangePasswordPage from "./routes/user/change-password";
import LibraryBrowsePage from "./routes/library/browse";
import LibraryCatalogPage from "./routes/library/catalog";
import LibraryShowPage from "./routes/library/show";
import SearchPage from "./routes/search";
import PlayerPage from "./pages/player";
import AdminLibrariesPage from "./pages/admin/libraries";
import AdminLibraryDetailPage from "./pages/admin/libraries/detail";
import AdminUsersPage from "./pages/admin/users/index";
import AdminUserDetailPage from "./pages/admin/users/detail";
import AdminUserLibrariesPage from "./pages/admin/users/libraries";
import AdminSchedulePage from "./pages/admin/schedule";
import AdminSettingsPage from "./pages/admin/settings/index";
import AdminTrashPage from "./pages/admin/trash";

export const router = createBrowserRouter([
  {
    element: <GuestRoute />,
    children: [
      { path: "/login", element: <LoginPage /> },
      { path: "/login/2fa", element: <LoginTwoFactorPage /> },
      { path: "/forgot-password", element: <ForgotPasswordPage /> },
      { path: "/reset-password", element: <ResetPasswordPage /> },
      { path: "/confirm-email", element: <ConfirmEmailPage /> },
      { path: "/accept-invite", element: <AcceptInvitePage /> },
      { path: "/invite", element: <AcceptInvitePage /> },
    ],
  },
  {
    element: <ProtectedRoute />,
    children: [
      { path: "/user/change-password", element: <UserChangePasswordPage /> },
      {
        path: "/",
        element: <DashboardLayout />,
        children: [
          { index: true, element: <HomePage /> },
          { path: "continue", element: <ContinuePage /> },
          { path: "favorites", element: <FavoritesPage /> },
          { path: "watched", element: <WatchedPage /> },
          { path: "unwatched", element: <UnwatchedPage /> },
          {
            path: "user",
            element: <UserLayout />,
            children: [{ index: true, element: <UserIndexPage /> }],
          },
          { path: "account", element: <Navigate to="/user" replace /> },
          { path: "account/*", element: <Navigate to="/user" replace /> },
          { path: "libraries", element: <LibrariesPage /> },
          { path: "search", element: <SearchPage /> },
          { path: "libraries/:slug", element: <LibraryCatalogPage /> },
          { path: "libraries/:slug/series/:showKey", element: <LibraryShowPage /> },
          { path: "libraries/:slug/browse/*", element: <LibraryBrowsePage /> },
          {
            element: <AdminRoute />,
            children: [
              { path: "admin/libraries", element: <AdminLibrariesPage /> },
              { path: "admin/libraries/:id", element: <AdminLibraryDetailPage /> },
              { path: "admin/schedule", element: <AdminSchedulePage /> },
              { path: "admin/users", element: <AdminUsersPage /> },
              { path: "admin/users/:id", element: <AdminUserDetailPage /> },
              { path: "admin/users/:id/libraries", element: <AdminUserLibrariesPage /> },
              { path: "admin/trash", element: <AdminTrashPage /> },
              { path: "admin/invites", element: <Navigate to="/admin/users" replace /> },
              { path: "admin/settings", element: <AdminSettingsPage /> },
            ],
          },
        ],
      },
      { path: "play/*", element: <PlayerPage /> },
    ],
  },
]);
